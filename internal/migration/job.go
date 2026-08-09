package migration

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/hkjang/kanvas/internal/store"
	"github.com/jackc/pgx/v5"
)

type Service struct {
	Store   *store.Store
	mu      sync.Mutex
	cancels map[uuid.UUID]context.CancelFunc
}

type SnapshotOptions struct {
	BatchSize                 int  `json:"batchSize"`
	IncludeUsers              bool `json:"includeUsers"`
	IncludeGroups             bool `json:"includeGroups"`
	IncludeSpaces             bool `json:"includeSpaces"`
	IncludePages              bool `json:"includePages"`
	IncludeComments           bool `json:"includeComments"`
	IncludeAttachmentMetadata bool `json:"includeAttachmentMetadata"`
	IncludePermissions        bool `json:"includePermissions"`
}

func DefaultSnapshotOptions() SnapshotOptions {
	return SnapshotOptions{BatchSize: 500, IncludeUsers: true, IncludeGroups: true, IncludeSpaces: true, IncludePages: true, IncludeComments: true, IncludeAttachmentMetadata: true, IncludePermissions: true}
}

type Job struct {
	ID              uuid.UUID       `json:"id"`
	Kind            string          `json:"kind"`
	Status          string          `json:"status"`
	TotalItems      int64           `json:"totalItems"`
	ProcessedItems  int64           `json:"processedItems"`
	FailedItems     int64           `json:"failedItems"`
	Checkpoint      json.RawMessage `json:"checkpoint"`
	Options         json.RawMessage `json:"options"`
	CurrentEntity   string          `json:"currentEntity"`
	CancelRequested bool            `json:"cancelRequested"`
	Error           string          `json:"error"`
	CreatedBy       *uuid.UUID      `json:"createdBy,omitempty"`
	CreatedAt       time.Time       `json:"createdAt"`
	StartedAt       *time.Time      `json:"startedAt,omitempty"`
	FinishedAt      *time.Time      `json:"finishedAt,omitempty"`
}

type MigrationItem struct {
	ID         uuid.UUID       `json:"id"`
	JobID      uuid.UUID       `json:"jobId"`
	EntityType string          `json:"entityType"`
	LegacyID   string          `json:"legacyId"`
	TargetID   *uuid.UUID      `json:"targetId,omitempty"`
	Status     string          `json:"status"`
	RetryCount int             `json:"retryCount"`
	Error      string          `json:"error"`
	Details    json.RawMessage `json:"details"`
	FinishedAt *time.Time      `json:"finishedAt,omitempty"`
}

func (s *Service) StartSnapshot(ctx context.Context, dsn string, actor uuid.UUID, options SnapshotOptions) (Job, error) {
	if dsn == "" {
		return Job{}, errors.New("Confluence DSN is not configured")
	}
	var discoveries int
	if err := s.Store.Pool.QueryRow(ctx, `SELECT count(*) FROM schema_discovery`).Scan(&discoveries); err != nil {
		return Job{}, err
	}
	if discoveries == 0 {
		return Job{}, errors.New("schema discovery must complete before snapshot")
	}
	var phase string
	if err := s.Store.Pool.QueryRow(ctx, `SELECT phase FROM migration_state WHERE id=true`).Scan(&phase); err != nil {
		return Job{}, err
	}
	if phase != "DISCOVERY" && phase != "SNAPSHOT" && phase != "ERROR" {
		return Job{}, fmt.Errorf("snapshot cannot start while migration phase is %s", phase)
	}
	var active int
	if err := s.Store.Pool.QueryRow(ctx, `SELECT count(*) FROM migration_jobs WHERE kind='SNAPSHOT' AND status IN ('PENDING','RUNNING','CANCEL_REQUESTED')`).Scan(&active); err != nil {
		return Job{}, err
	}
	if active > 0 {
		return Job{}, errors.New("a snapshot job is already active")
	}
	if options.BatchSize < 10 || options.BatchSize > 5000 {
		options.BatchSize = 500
	}
	if !options.IncludeUsers && !options.IncludeGroups && !options.IncludeSpaces && !options.IncludePages && !options.IncludeComments && !options.IncludeAttachmentMetadata && !options.IncludePermissions {
		options = DefaultSnapshotOptions()
	}
	source, err := openLegacy(ctx, dsn)
	if err != nil {
		return Job{}, err
	}
	total, err := source.countSnapshotItems(ctx, options)
	source.close()
	if err != nil {
		return Job{}, err
	}
	id := uuid.New()
	raw, _ := json.Marshal(options)
	_, err = s.Store.Pool.Exec(ctx, `INSERT INTO migration_jobs(id,kind,status,total_items,options,created_by) VALUES($1,'SNAPSHOT','PENDING',$2,$3,$4)`, id, total, raw, actor)
	if err != nil {
		return Job{}, err
	}
	_, _ = s.Store.Pool.Exec(ctx, `UPDATE migration_state SET phase='SNAPSHOT',started_at=coalesce(started_at,now()),updated_at=now(),details=details||jsonb_build_object('activeJobId',$1::uuid) WHERE id=true`, id)
	runCtx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	if s.cancels == nil {
		s.cancels = map[uuid.UUID]context.CancelFunc{}
	}
	s.cancels[id] = cancel
	s.mu.Unlock()
	go func() {
		defer func() { s.mu.Lock(); delete(s.cancels, id); s.mu.Unlock() }()
		s.runSnapshot(runCtx, dsn, id, actor, options)
	}()
	s.Store.Audit(ctx, &actor, "MIGRATION_SNAPSHOT_START", "MIGRATION_JOB", id.String(), "", "", map[string]any{"totalItems": total, "options": options})
	return s.Job(ctx, id)
}

func (s *Service) CancelJob(ctx context.Context, id, actor uuid.UUID) error {
	cmd, err := s.Store.Pool.Exec(ctx, `UPDATE migration_jobs SET cancel_requested=true,status=CASE WHEN status IN ('PENDING','RUNNING') THEN 'CANCEL_REQUESTED' ELSE status END WHERE id=$1 AND status IN ('PENDING','RUNNING','CANCEL_REQUESTED')`, id)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	s.mu.Lock()
	cancel := s.cancels[id]
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	s.Store.Audit(ctx, &actor, "MIGRATION_JOB_CANCEL", "MIGRATION_JOB", id.String(), "", "", map[string]any{})
	return nil
}

func (s *Service) RecoverInterruptedJobs(ctx context.Context) error {
	_, err := s.Store.Pool.Exec(ctx, `UPDATE migration_jobs SET status='INTERRUPTED',error='Kanvas restarted while this job was active',finished_at=now(),current_entity='' WHERE status IN ('PENDING','RUNNING','CANCEL_REQUESTED')`)
	return err
}

func (s *Service) ResumeSnapshot(ctx context.Context, dsn string, jobID, actor uuid.UUID) (Job, error) {
	job, err := s.Job(ctx, jobID)
	if err != nil {
		return Job{}, err
	}
	if job.Kind != "SNAPSHOT" {
		return Job{}, errors.New("only snapshot jobs can be resumed")
	}
	allowed := map[string]bool{"FAILED": true, "INTERRUPTED": true, "CANCELLED": true, "COMPLETED_WITH_ERRORS": true}
	if !allowed[job.Status] {
		return Job{}, fmt.Errorf("job in status %s cannot be resumed", job.Status)
	}
	var phase string
	if err = s.Store.Pool.QueryRow(ctx, `SELECT phase FROM migration_state WHERE id=true`).Scan(&phase); err != nil {
		return Job{}, err
	}
	if phase != "DISCOVERY" && phase != "SNAPSHOT" && phase != "ERROR" {
		return Job{}, fmt.Errorf("snapshot cannot resume while migration phase is %s", phase)
	}
	var active int
	if err = s.Store.Pool.QueryRow(ctx, `SELECT count(*) FROM migration_jobs WHERE id<>$1 AND kind='SNAPSHOT' AND status IN ('PENDING','RUNNING','CANCEL_REQUESTED')`, jobID).Scan(&active); err != nil {
		return Job{}, err
	}
	if active > 0 {
		return Job{}, errors.New("a snapshot job is already active")
	}
	options := DefaultSnapshotOptions()
	if len(job.Options) > 0 {
		_ = json.Unmarshal(job.Options, &options)
	}
	_, err = s.Store.Pool.Exec(ctx, `UPDATE migration_jobs SET status='PENDING',cancel_requested=false,error='',finished_at=NULL WHERE id=$1`, jobID)
	if err != nil {
		return Job{}, err
	}
	runCtx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	if s.cancels == nil {
		s.cancels = map[uuid.UUID]context.CancelFunc{}
	}
	s.cancels[jobID] = cancel
	s.mu.Unlock()
	go func() {
		defer func() { s.mu.Lock(); delete(s.cancels, jobID); s.mu.Unlock() }()
		s.runSnapshot(runCtx, dsn, jobID, actor, options)
	}()
	s.Store.Audit(ctx, &actor, "MIGRATION_JOB_RESUME", "MIGRATION_JOB", jobID.String(), "", "", map[string]any{})
	return s.Job(ctx, jobID)
}

func (s *Service) Job(ctx context.Context, id uuid.UUID) (Job, error) {
	return scanJob(s.Store.Pool.QueryRow(ctx, jobSelect+` WHERE id=$1`, id))
}
func (s *Service) Jobs(ctx context.Context, limit int) ([]Job, error) {
	if limit < 1 || limit > 200 {
		limit = 50
	}
	rows, err := s.Store.Pool.Query(ctx, jobSelect+` ORDER BY created_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Job, 0)
	for rows.Next() {
		v, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Service) JobItems(ctx context.Context, jobID uuid.UUID, status string, limit int) ([]MigrationItem, error) {
	if limit < 1 || limit > 1000 {
		limit = 200
	}
	query := `SELECT id,job_id,entity_type,legacy_id,target_id,status,retry_count,error,details,finished_at FROM migration_items WHERE job_id=$1`
	args := []any{jobID}
	if status != "" {
		query += ` AND status=$2`
		args = append(args, status)
	}
	query += fmt.Sprintf(` ORDER BY entity_type,legacy_id LIMIT $%d`, len(args)+1)
	args = append(args, limit)
	rows, err := s.Store.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]MigrationItem, 0)
	for rows.Next() {
		var v MigrationItem
		if err := rows.Scan(&v.ID, &v.JobID, &v.EntityType, &v.LegacyID, &v.TargetID, &v.Status, &v.RetryCount, &v.Error, &v.Details, &v.FinishedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

const jobSelect = `SELECT id,kind,status,total_items,processed_items,failed_items,checkpoint,options,current_entity,cancel_requested,error,created_by,created_at,started_at,finished_at FROM migration_jobs`

type rowScanner interface{ Scan(...any) error }

func scanJob(row rowScanner) (Job, error) {
	var v Job
	err := row.Scan(&v.ID, &v.Kind, &v.Status, &v.TotalItems, &v.ProcessedItems, &v.FailedItems, &v.Checkpoint, &v.Options, &v.CurrentEntity, &v.CancelRequested, &v.Error, &v.CreatedBy, &v.CreatedAt, &v.StartedAt, &v.FinishedAt)
	return v, err
}

func openLegacy(ctx context.Context, dsn string) (*legacySource, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(5 * time.Minute)
	pingCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err = db.PingContext(pingCtx); err != nil {
		db.Close()
		return nil, fmt.Errorf("connect legacy MySQL: %w", err)
	}
	return &legacySource{db: db, columnCache: map[string]map[string]bool{}}, nil
}
