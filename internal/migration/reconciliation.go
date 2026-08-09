package migration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type ReconciliationOptions struct {
	SnapshotJobID uuid.UUID `json:"snapshotJobId"`
}

func (s *Service) StartReconciliation(ctx context.Context, actor uuid.UUID) (Job, error) {
	snapshot, err := scanJob(s.Store.Pool.QueryRow(ctx, jobSelect+` WHERE kind='SNAPSHOT' AND status IN ('COMPLETE','COMPLETED_WITH_ERRORS') ORDER BY created_at DESC LIMIT 1`))
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, errors.New("a completed snapshot is required before reconciliation")
	}
	if err != nil {
		return Job{}, err
	}
	var active int
	if err = s.Store.Pool.QueryRow(ctx, `SELECT count(*) FROM migration_jobs WHERE kind='RECONCILIATION' AND status IN ('PENDING','RUNNING','CANCEL_REQUESTED')`).Scan(&active); err != nil {
		return Job{}, err
	}
	if active > 0 {
		return Job{}, errors.New("a reconciliation job is already active")
	}
	jobID := uuid.New()
	options, _ := json.Marshal(ReconciliationOptions{SnapshotJobID: snapshot.ID})
	_, err = s.Store.Pool.Exec(ctx, `INSERT INTO migration_jobs(id,kind,status,total_items,options,current_entity,created_by) VALUES($1,'RECONCILIATION','PENDING',13,$2,'PREPARING',$3)`, jobID, options, actor)
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
		defer func() {
			s.mu.Lock()
			delete(s.cancels, jobID)
			s.mu.Unlock()
		}()
		s.runReconciliation(runCtx, jobID, snapshot, actor)
	}()
	s.Store.Audit(ctx, &actor, "MIGRATION_RECONCILIATION_START", "MIGRATION_JOB", jobID.String(), "", "", map[string]any{"snapshotJobId": snapshot.ID})
	return s.Job(ctx, jobID)
}

func (s *Service) runReconciliation(ctx context.Context, jobID uuid.UUID, snapshot Job, actor uuid.UUID) {
	_, _ = s.Store.Pool.Exec(context.Background(), `UPDATE migration_jobs SET status='RUNNING',current_entity='RECONCILIATION',started_at=now() WHERE id=$1`, jobID)
	options := DefaultSnapshotOptions()
	if len(snapshot.Options) > 0 {
		_ = json.Unmarshal(snapshot.Options, &options)
	}
	runner := &snapshotRunner{service: s, jobID: snapshot.ID, options: options}
	err := runner.reconcile(ctx)
	if err != nil {
		status := "FAILED"
		if errors.Is(err, context.Canceled) {
			status = "CANCELLED"
		}
		_, _ = s.Store.Pool.Exec(context.Background(), `UPDATE migration_jobs SET status=$2,error=$3,current_entity='',finished_at=now() WHERE id=$1`, jobID, status, truncate(err.Error(), 4000))
		_, _ = s.Store.Pool.Exec(context.Background(), `UPDATE migration_state SET details=details||jsonb_build_object('lastReconciliationError',$1::text),updated_at=now() WHERE id=true`, truncate(err.Error(), 4000))
		s.Store.Audit(context.Background(), &actor, "MIGRATION_RECONCILIATION_FINISH", "MIGRATION_JOB", jobID.String(), "", "", map[string]any{"status": status, "error": truncate(err.Error(), 1000)})
		return
	}
	var checks int64
	var readiness float64
	_ = s.Store.Pool.QueryRow(context.Background(), `SELECT count(*) FROM migration_checks`).Scan(&checks)
	_ = s.Store.Pool.QueryRow(context.Background(), `SELECT readiness FROM migration_state WHERE id=true`).Scan(&readiness)
	checkpoint, _ := json.Marshal(map[string]any{"snapshotJobId": snapshot.ID, "checks": checks, "readiness": readiness})
	_, err = s.Store.Pool.Exec(context.Background(), `UPDATE migration_jobs SET status='COMPLETE',total_items=$2,processed_items=$2,current_entity='',checkpoint=$3,finished_at=now() WHERE id=$1`, jobID, checks, checkpoint)
	if err != nil {
		_, _ = s.Store.Pool.Exec(context.Background(), `UPDATE migration_jobs SET status='FAILED',error=$2,current_entity='',finished_at=now() WHERE id=$1`, jobID, truncate(fmt.Sprintf("finish reconciliation: %v", err), 4000))
		return
	}
	_, _ = s.Store.Pool.Exec(context.Background(), `UPDATE migration_state SET details=details||jsonb_build_object('lastReconciliationJobId',$1::uuid,'lastReconciliationAt',$2::timestamptz),updated_at=now() WHERE id=true`, jobID, time.Now().UTC())
	s.Store.Audit(context.Background(), &actor, "MIGRATION_RECONCILIATION_FINISH", "MIGRATION_JOB", jobID.String(), "", "", map[string]any{"status": "COMPLETE", "snapshotJobId": snapshot.ID, "checks": checks, "readiness": readiness})
}
