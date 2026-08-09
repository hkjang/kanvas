package migration

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/hkjang/kanvas/internal/store"
)

func TestUnsupportedDecisionIntegration(t *testing.T) {
	dsn := os.Getenv("KANVAS_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("KANVAS_TEST_POSTGRES_DSN is not set")
	}
	ctx := context.Background()
	st, err := store.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	var actor uuid.UUID
	if err = st.Pool.QueryRow(ctx, `SELECT id FROM users WHERE role='ADMIN' AND status='ACTIVE' ORDER BY created_at LIMIT 1`).Scan(&actor); err != nil {
		t.Fatal(err)
	}
	jobID := uuid.New()
	itemIDs := []uuid.UUID{uuid.New(), uuid.New()}
	if _, err = st.Pool.Exec(ctx, `INSERT INTO migration_jobs(id,kind,status,options,created_by,started_at,finished_at) VALUES($1,'SNAPSHOT','COMPLETE','{"includePages":true}'::jsonb,$2,now(),now())`, jobID, actor); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = st.Pool.Exec(context.Background(), `DELETE FROM migration_checks WHERE details->>'jobId'=$1`, jobID.String())
		_, _ = st.Pool.Exec(context.Background(), `DELETE FROM migration_jobs WHERE id=$1`, jobID)
	})
	if _, err = st.Pool.Exec(ctx, `INSERT INTO unsupported_content(id,job_id,legacy_id,kind,name,occurrence_count,sample) VALUES($1,$3,'page-1','UNKNOWN_MACRO','custom-macro',4,'sample'),($2,$3,'page-2','ORPHAN_PAGE','missing-parent',1,'sample')`, itemIDs[0], itemIDs[1], jobID); err != nil {
		t.Fatal(err)
	}
	service := &Service{Store: st}
	updated, err := service.DecideUnsupportedContent(ctx, itemIDs, "APPROVED", "integration test risk approval", actor)
	if err != nil {
		t.Fatal(err)
	}
	if updated != 2 {
		t.Fatalf("updated = %d, want 2", updated)
	}
	page, err := service.UnsupportedContent(ctx, UnsupportedFilter{JobID: &jobID, Status: "APPROVED", Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if page.Summary.Approved != 2 || page.FilteredTotal != 2 || len(page.Items) != 2 {
		t.Fatalf("unexpected page: %#v", page)
	}
	var passing int
	if err = st.Pool.QueryRow(ctx, `SELECT count(*) FROM migration_checks WHERE details->>'jobId'=$1 AND status='APPROVED'`, jobID.String()).Scan(&passing); err != nil {
		t.Fatal(err)
	}
	if passing != 2 {
		t.Fatalf("approved checks = %d, want 2", passing)
	}
}
