package migration

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type UnsupportedItem struct {
	ID              uuid.UUID  `json:"id"`
	JobID           *uuid.UUID `json:"jobId,omitempty"`
	PageID          *uuid.UUID `json:"pageId,omitempty"`
	LegacyID        string     `json:"legacyId"`
	Kind            string     `json:"kind"`
	Name            string     `json:"name"`
	Status          string     `json:"status"`
	OccurrenceCount int64      `json:"occurrenceCount"`
	Sample          string     `json:"sample"`
	Resolution      string     `json:"resolution"`
	ResolvedBy      *uuid.UUID `json:"resolvedBy,omitempty"`
	ResolvedAt      *time.Time `json:"resolvedAt,omitempty"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
}

type UnsupportedFilter struct {
	JobID  *uuid.UUID
	Status string
	Kind   string
	Query  string
	Limit  int
	Offset int
}

type UnsupportedSummary struct {
	Total    int64            `json:"total"`
	Open     int64            `json:"open"`
	Approved int64            `json:"approved"`
	Resolved int64            `json:"resolved"`
	ByKind   map[string]int64 `json:"byKind"`
}

type UnsupportedPage struct {
	Items         []UnsupportedItem  `json:"items"`
	Summary       UnsupportedSummary `json:"summary"`
	SnapshotJobID *uuid.UUID         `json:"snapshotJobId,omitempty"`
	FilteredTotal int64              `json:"filteredTotal"`
	Limit         int                `json:"limit"`
	Offset        int                `json:"offset"`
}

func (s *Service) UnsupportedContent(ctx context.Context, filter UnsupportedFilter) (UnsupportedPage, error) {
	filter.Status = strings.ToUpper(strings.TrimSpace(filter.Status))
	filter.Kind = strings.ToUpper(strings.TrimSpace(filter.Kind))
	filter.Query = strings.TrimSpace(filter.Query)
	if filter.Status != "" && !validUnsupportedStatus(filter.Status) {
		return UnsupportedPage{}, errors.New("unsupported content status must be OPEN, APPROVED, or RESOLVED")
	}
	if filter.Limit < 1 || filter.Limit > 200 {
		filter.Limit = 100
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	selectedJob := filter.JobID
	if selectedJob == nil {
		var latest uuid.UUID
		err := s.Store.Pool.QueryRow(ctx, `SELECT id FROM migration_jobs WHERE kind='SNAPSHOT' ORDER BY created_at DESC LIMIT 1`).Scan(&latest)
		if err == nil {
			selectedJob = &latest
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return UnsupportedPage{}, err
		}
	}
	result := UnsupportedPage{
		Items:         make([]UnsupportedItem, 0),
		Summary:       UnsupportedSummary{ByKind: map[string]int64{}},
		SnapshotJobID: selectedJob,
		Limit:         filter.Limit,
		Offset:        filter.Offset,
	}
	if err := s.Store.Pool.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE status='OPEN'),count(*) FILTER(WHERE status='APPROVED'),count(*) FILTER(WHERE status='RESOLVED') FROM unsupported_content WHERE ($1::uuid IS NULL OR job_id=$1)`, selectedJob).Scan(&result.Summary.Total, &result.Summary.Open, &result.Summary.Approved, &result.Summary.Resolved); err != nil {
		return result, err
	}
	kindRows, err := s.Store.Pool.Query(ctx, `SELECT kind,count(*) FROM unsupported_content WHERE ($1::uuid IS NULL OR job_id=$1) GROUP BY kind ORDER BY kind`, selectedJob)
	if err != nil {
		return result, err
	}
	for kindRows.Next() {
		var kind string
		var count int64
		if err = kindRows.Scan(&kind, &count); err != nil {
			kindRows.Close()
			return result, err
		}
		result.Summary.ByKind[kind] = count
	}
	if err = kindRows.Err(); err != nil {
		kindRows.Close()
		return result, err
	}
	kindRows.Close()
	if err = s.Store.Pool.QueryRow(ctx, `SELECT count(*) FROM unsupported_content
		WHERE ($1::uuid IS NULL OR job_id=$1) AND ($2='' OR status=$2) AND ($3='' OR kind=$3)
		AND ($4='' OR kind ILIKE '%'||$4||'%' OR name ILIKE '%'||$4||'%' OR sample ILIKE '%'||$4||'%' OR legacy_id ILIKE '%'||$4||'%')`, selectedJob, filter.Status, filter.Kind, filter.Query).Scan(&result.FilteredTotal); err != nil {
		return result, err
	}
	rows, err := s.Store.Pool.Query(ctx, `SELECT id,job_id,page_id,legacy_id,kind,name,status,occurrence_count,sample,resolution,resolved_by,resolved_at,created_at,updated_at
		FROM unsupported_content
		WHERE ($1::uuid IS NULL OR job_id=$1) AND ($2='' OR status=$2) AND ($3='' OR kind=$3)
		AND ($4='' OR kind ILIKE '%'||$4||'%' OR name ILIKE '%'||$4||'%' OR sample ILIKE '%'||$4||'%' OR legacy_id ILIKE '%'||$4||'%')
		ORDER BY CASE status WHEN 'OPEN' THEN 0 WHEN 'APPROVED' THEN 1 ELSE 2 END,occurrence_count DESC,kind,name
		LIMIT $5 OFFSET $6`, selectedJob, filter.Status, filter.Kind, filter.Query, filter.Limit, filter.Offset)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var item UnsupportedItem
		if err = rows.Scan(&item.ID, &item.JobID, &item.PageID, &item.LegacyID, &item.Kind, &item.Name, &item.Status, &item.OccurrenceCount, &item.Sample, &item.Resolution, &item.ResolvedBy, &item.ResolvedAt, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return result, err
		}
		result.Items = append(result.Items, item)
	}
	return result, rows.Err()
}

func (s *Service) DecideUnsupportedContent(ctx context.Context, ids []uuid.UUID, status, resolution string, actor uuid.UUID) (int64, error) {
	status = strings.ToUpper(strings.TrimSpace(status))
	resolution = strings.TrimSpace(resolution)
	if !validUnsupportedStatus(status) {
		return 0, errors.New("unsupported content status must be OPEN, APPROVED, or RESOLVED")
	}
	if len(ids) < 1 || len(ids) > 500 {
		return 0, errors.New("unsupported content decision requires between 1 and 500 items")
	}
	if status != "OPEN" && resolution == "" {
		return 0, errors.New("resolution is required when approving or resolving unsupported content")
	}
	if len([]rune(resolution)) > 2000 {
		return 0, errors.New("resolution must be 2000 characters or fewer")
	}
	tx, err := s.Store.Pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `UPDATE unsupported_content SET status=$2::text,resolution=$3::text,resolved_by=CASE WHEN $2::text='OPEN' THEN NULL ELSE $4::uuid END,resolved_at=CASE WHEN $2::text='OPEN' THEN NULL ELSE now() END,updated_at=now() WHERE id=ANY($1::uuid[]) RETURNING job_id`, ids, status, resolution, actor)
	if err != nil {
		return 0, err
	}
	jobs := map[uuid.UUID]bool{}
	var updated int64
	for rows.Next() {
		var jobID *uuid.UUID
		if err = rows.Scan(&jobID); err != nil {
			rows.Close()
			return 0, err
		}
		updated++
		if jobID != nil {
			jobs[*jobID] = true
		}
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()
	if updated != int64(len(ids)) {
		return 0, pgx.ErrNoRows
	}
	var latestSnapshot uuid.UUID
	latestErr := tx.QueryRow(ctx, `SELECT id FROM migration_jobs WHERE kind='SNAPSHOT' ORDER BY created_at DESC LIMIT 1`).Scan(&latestSnapshot)
	if latestErr != nil && !errors.Is(latestErr, pgx.ErrNoRows) {
		return 0, latestErr
	}
	if latestErr == nil && jobs[latestSnapshot] {
		if err = refreshUnsupportedChecks(ctx, tx, latestSnapshot); err != nil {
			return 0, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return 0, err
	}
	return updated, nil
}

func refreshUnsupportedChecks(ctx context.Context, tx pgx.Tx, snapshotJobID uuid.UUID) error {
	type counts struct{ open, approved int64 }
	read := func(kinds []string) (counts, error) {
		var value counts
		err := tx.QueryRow(ctx, `SELECT coalesce(sum(occurrence_count) FILTER(WHERE status='OPEN'),0),coalesce(sum(occurrence_count) FILTER(WHERE status='APPROVED'),0) FROM unsupported_content WHERE job_id=$1 AND kind=ANY($2)`, snapshotJobID, kinds).Scan(&value.open, &value.approved)
		return value, err
	}
	upsert := func(category, name string, value counts) error {
		openStatus := "WARNING"
		if category == "PAGE_HIERARCHY" {
			openStatus = "FAIL"
		}
		status := exceptionCheckStatus(value.open, value.approved, openStatus)
		_, err := tx.Exec(ctx, `INSERT INTO migration_checks(id,category,check_name,status,mismatch_count,details) VALUES($1,$2,$3,$4,$5,jsonb_build_object('jobId',$6::uuid,'approvedExceptions',$7::bigint))
			ON CONFLICT(category,check_name) DO UPDATE SET status=excluded.status,mismatch_count=excluded.mismatch_count,details=migration_checks.details||excluded.details,checked_at=now()`, uuid.New(), category, name, status, value.open, snapshotJobID, value.approved)
		return err
	}
	macroCounts, err := read([]string{"UNKNOWN_MACRO", "INVALID_XHTML", "UNKNOWN_PLUGIN_DATA"})
	if err != nil {
		return err
	}
	if err = upsert("MACROS", "Macro Compatibility", macroCounts); err != nil {
		return err
	}
	hierarchyCounts, err := read([]string{"ORPHAN_PAGE"})
	if err != nil {
		return err
	}
	if err = upsert("PAGE_HIERARCHY", "Page Tree", hierarchyCounts); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE migration_state SET readiness=round((SELECT count(*) FROM migration_checks WHERE status IN ('PASS','APPROVED'))::numeric/GREATEST((SELECT count(*) FROM migration_checks),13)*100,2),updated_at=now() WHERE id=true`)
	return err
}

func exceptionCheckStatus(open, approved int64, openStatus string) string {
	if open > 0 {
		return openStatus
	}
	if approved > 0 {
		return "APPROVED"
	}
	return "PASS"
}

func validUnsupportedStatus(status string) bool {
	return status == "OPEN" || status == "APPROVED" || status == "RESOLVED"
}
