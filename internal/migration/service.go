package migration

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var validIdentifier = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

type TableInfo struct {
	Name      string `json:"name"`
	Rows      int64  `json:"rows"`
	SizeBytes int64  `json:"sizeBytes"`
	Engine    string `json:"engine"`
	Category  string `json:"category"`
}

type DiscoveryResult struct {
	ID                uuid.UUID        `json:"id"`
	DatabaseVersion   string           `json:"databaseVersion"`
	CharacterSet      string           `json:"characterSet"`
	Collation         string           `json:"collation"`
	ConfluenceVersion string           `json:"confluenceVersion"`
	AttachmentMode    string           `json:"attachmentMode"`
	Tables            []TableInfo      `json:"tables"`
	CoreCounts        map[string]int64 `json:"coreCounts"`
	AOTables          int              `json:"aoTables"`
	UnknownTables     int              `json:"unknownTables"`
	CreatedAt         time.Time        `json:"createdAt"`
}

type Dashboard struct {
	Phase           string           `json:"phase"`
	SourceMode      string           `json:"sourceMode"`
	Readiness       float64          `json:"readiness"`
	CDCLagMS        int64            `json:"cdcLagMs"`
	FailedEvents    int64            `json:"failedEvents"`
	Details         json.RawMessage  `json:"details"`
	UpdatedAt       time.Time        `json:"updatedAt"`
	Checks          []Check          `json:"checks"`
	LatestDiscovery *DiscoveryResult `json:"latestDiscovery,omitempty"`
}

type Check struct {
	Category      string    `json:"category"`
	Name          string    `json:"name"`
	Status        string    `json:"status"`
	SourceCount   *int64    `json:"sourceCount,omitempty"`
	TargetCount   *int64    `json:"targetCount,omitempty"`
	MismatchCount int64     `json:"mismatchCount"`
	CheckedAt     time.Time `json:"checkedAt"`
}

var transitions = map[string]map[string]bool{
	"LEGACY": {"DISCOVERY": true}, "DISCOVERY": {"SNAPSHOT": true, "ERROR": true, "LEGACY": true}, "SNAPSHOT": {"CDC_SYNC": true, "ERROR": true}, "CDC_SYNC": {"VERIFY": true, "ERROR": true}, "VERIFY": {"SHADOW": true, "ERROR": true}, "SHADOW": {"CUTOVER_READY": true, "ERROR": true}, "CUTOVER_READY": {"FREEZE": true, "LEGACY": true}, "FREEZE": {"FINAL_SYNC": true, "WINBACK": true}, "FINAL_SYNC": {"CUTOVER": true, "WINBACK": true, "ERROR": true}, "CUTOVER": {"STABILIZING": true, "WINBACK": true}, "STABILIZING": {"COMPLETE": true, "WINBACK": true}, "WINBACK": {"LEGACY": true}, "ERROR": {"LEGACY": true, "DISCOVERY": true},
}

func (s *Service) Transition(ctx context.Context, target string, actor uuid.UUID) error {
	target = strings.ToUpper(strings.TrimSpace(target))
	tx, err := s.Store.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('kanvas_migration_transition'))`); err != nil {
		return err
	}
	var current string
	if err = tx.QueryRow(ctx, `SELECT phase FROM migration_state WHERE id=true FOR UPDATE`).Scan(&current); err != nil {
		return err
	}
	if !transitions[current][target] {
		return fmt.Errorf("invalid migration transition %s -> %s", current, target)
	}
	if err = s.transitionEvidence(ctx, tx, current, target); err != nil {
		return err
	}
	if target == "CUTOVER_READY" || target == "CUTOVER" {
		var total, failures int
		if err = tx.QueryRow(ctx, `SELECT count(*),count(*) FILTER (WHERE status NOT IN ('PASS','APPROVED')) FROM migration_checks`).Scan(&total, &failures); err != nil {
			return err
		}
		if total < 13 {
			return fmt.Errorf("cutover gate rejected: only %d of 13 required evidence checks exist", total)
		}
		if failures > 0 {
			return fmt.Errorf("cutover gate rejected: %d checks are not passing", failures)
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE migration_state SET phase=$1,source_mode=CASE WHEN $1='CUTOVER' THEN 'POSTGRES' WHEN $1='SHADOW' THEN 'SHADOW' WHEN $1 IN ('LEGACY','WINBACK') THEN 'LEGACY' ELSE source_mode END,updated_at=now() WHERE id=true`, target); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	s.Store.Audit(ctx, &actor, "MIGRATION_TRANSITION", "MIGRATION", "singleton", "", "", map[string]any{"from": current, "to": target})
	return nil
}

type evidenceQueryer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func (s *Service) transitionEvidence(ctx context.Context, queryer evidenceQueryer, current, target string) error {
	if target == "CDC_SYNC" {
		return errors.New("CDC engine is not implemented in this release; transition is fail-closed")
	}
	if current == "DISCOVERY" && target == "SNAPSHOT" {
		var n int
		if err := queryer.QueryRow(ctx, `SELECT count(*) FROM schema_discovery`).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			return errors.New("schema discovery evidence is required before snapshot")
		}
	}
	requiredJob := map[string]string{"CDC_SYNC": "SNAPSHOT", "VERIFY": "CDC", "SHADOW": "VALIDATION", "CUTOVER_READY": "SHADOW", "FINAL_SYNC": "FREEZE", "CUTOVER": "FINAL_SYNC", "STABILIZING": "CUTOVER"}[target]
	if requiredJob != "" {
		var n int
		if err := queryer.QueryRow(ctx, `SELECT count(*) FROM migration_jobs WHERE kind=$1 AND status='COMPLETE'`, requiredJob).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			return fmt.Errorf("%s job evidence is required before entering %s", requiredJob, target)
		}
	}
	return nil
}

func (s *Service) Discover(ctx context.Context, dsn string, actor uuid.UUID) (result DiscoveryResult, err error) {
	if strings.TrimSpace(dsn) == "" {
		return result, errors.New("Confluence DSN is not configured")
	}
	result.ID = uuid.New()
	result.AttachmentMode = "filesystem"
	result.CoreCounts = map[string]int64{}
	result.CreatedAt = time.Now()
	_, _ = s.Store.Pool.Exec(ctx, `UPDATE migration_state SET phase='DISCOVERY',updated_at=now() WHERE id=true AND phase IN ('LEGACY','ERROR','DISCOVERY')`)
	defer func() {
		if err != nil {
			_, _ = s.Store.Pool.Exec(context.Background(), `UPDATE migration_state SET phase='ERROR',updated_at=now(),details=jsonb_build_object('lastError',$1::text) WHERE id=true`, err.Error())
		}
	}()
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return result, err
	}
	defer db.Close()
	db.SetMaxOpenConns(3)
	db.SetConnMaxLifetime(2 * time.Minute)
	checkCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	if err = db.PingContext(checkCtx); err != nil {
		return result, fmt.Errorf("connect legacy MySQL: %w", err)
	}
	_ = db.QueryRowContext(checkCtx, `SELECT VERSION()`).Scan(&result.DatabaseVersion)
	_ = db.QueryRowContext(checkCtx, `SELECT @@character_set_database,@@collation_database`).Scan(&result.CharacterSet, &result.Collation)
	rows, err := db.QueryContext(checkCtx, `SELECT TABLE_NAME,COALESCE(TABLE_ROWS,0),COALESCE(DATA_LENGTH,0)+COALESCE(INDEX_LENGTH,0),COALESCE(ENGINE,'') FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() ORDER BY TABLE_NAME`)
	if err != nil {
		return result, err
	}
	known := knownTables()
	for rows.Next() {
		var t TableInfo
		if err = rows.Scan(&t.Name, &t.Rows, &t.SizeBytes, &t.Engine); err != nil {
			rows.Close()
			return result, err
		}
		upper := strings.ToUpper(t.Name)
		switch {
		case strings.HasPrefix(upper, "AO_"):
			t.Category = "PLUGIN"
			result.AOTables++
		case known[upper]:
			t.Category = "CORE"
		default:
			t.Category = "UNKNOWN"
			result.UnknownTables++
		}
		result.Tables = append(result.Tables, t)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return result, err
	}
	for _, table := range []string{"CONTENT", "BODYCONTENT", "SPACES", "CONTENT_PERM", "CONTENT_PERM_SET", "CONTENTPROPERTIES", "CWD_USER", "CWD_GROUP", "CWD_MEMBERSHIP", "ATTACHMENTS", "NOTIFICATIONS", "LABEL"} {
		if !validIdentifier.MatchString(table) || !tableExists(result.Tables, table) {
			continue
		}
		var count int64
		if e := db.QueryRowContext(checkCtx, "SELECT COUNT(*) FROM `"+table+"`").Scan(&count); e == nil {
			result.CoreCounts[table] = count
		}
	}
	for _, q := range []string{`SELECT COALESCE(MAX(BUILDNUMBER),'') FROM CONFVERSION`, `SELECT COALESCE(MAX(VERSION),'') FROM CONFVERSION`} {
		if e := db.QueryRowContext(checkCtx, q).Scan(&result.ConfluenceVersion); e == nil && result.ConfluenceVersion != "" {
			break
		}
	}
	sort.Slice(result.Tables, func(i, j int) bool { return result.Tables[i].Name < result.Tables[j].Name })
	summary, _ := json.Marshal(map[string]any{"tables": result.Tables, "coreCounts": result.CoreCounts, "aoTables": result.AOTables, "unknownTables": result.UnknownTables})
	_, err = s.Store.Pool.Exec(ctx, `INSERT INTO schema_discovery(id,database_version,character_set,collation_name,confluence_version,attachment_mode,summary,created_by,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, result.ID, result.DatabaseVersion, result.CharacterSet, result.Collation, result.ConfluenceVersion, result.AttachmentMode, summary, actor, result.CreatedAt)
	if err != nil {
		return result, err
	}
	checks := []struct {
		category, name string
		source, target int64
	}{{"DATABASE", "Legacy MySQL connection", 1, 1}, {"SCHEMA", "Schema discovery", int64(len(result.Tables)), int64(len(result.Tables))}}
	for _, c := range checks {
		_, err = s.Store.Pool.Exec(ctx, `INSERT INTO migration_checks(id,category,check_name,status,source_count,target_count,mismatch_count) VALUES($1,$2,$3,'PASS',$4,$5,0) ON CONFLICT(category,check_name) DO UPDATE SET status='PASS',source_count=excluded.source_count,target_count=excluded.target_count,mismatch_count=0,checked_at=now()`, uuid.New(), c.category, c.name, c.source, c.target)
		if err != nil {
			return result, err
		}
	}
	_, _ = s.Store.Pool.Exec(ctx, `UPDATE migration_state SET readiness=round((SELECT count(*) FROM migration_checks WHERE status IN ('PASS','APPROVED'))::numeric/13*100,2),details=details||jsonb_build_object('lastDiscoveryId',$1::uuid),updated_at=now() WHERE id=true`, result.ID)
	s.Store.Audit(ctx, &actor, "SCHEMA_DISCOVERY", "MIGRATION", result.ID.String(), "", "", map[string]any{"tables": len(result.Tables), "aoTables": result.AOTables})
	return result, nil
}

func (s *Service) Dashboard(ctx context.Context) (Dashboard, error) {
	var d Dashboard
	if err := s.Store.Pool.QueryRow(ctx, `SELECT phase,source_mode,readiness,cdc_lag_ms,failed_events,details,updated_at FROM migration_state WHERE id=true`).Scan(&d.Phase, &d.SourceMode, &d.Readiness, &d.CDCLagMS, &d.FailedEvents, &d.Details, &d.UpdatedAt); err != nil {
		return d, err
	}
	rows, err := s.Store.Pool.Query(ctx, `SELECT category,check_name,status,source_count,target_count,mismatch_count,checked_at FROM migration_checks ORDER BY category,check_name`)
	if err != nil {
		return d, err
	}
	for rows.Next() {
		var c Check
		if err = rows.Scan(&c.Category, &c.Name, &c.Status, &c.SourceCount, &c.TargetCount, &c.MismatchCount, &c.CheckedAt); err != nil {
			rows.Close()
			return d, err
		}
		d.Checks = append(d.Checks, c)
	}
	rows.Close()
	var x DiscoveryResult
	var summary json.RawMessage
	err = s.Store.Pool.QueryRow(ctx, `SELECT id,database_version,character_set,collation_name,confluence_version,attachment_mode,summary,created_at FROM schema_discovery ORDER BY created_at DESC LIMIT 1`).Scan(&x.ID, &x.DatabaseVersion, &x.CharacterSet, &x.Collation, &x.ConfluenceVersion, &x.AttachmentMode, &summary, &x.CreatedAt)
	if err == nil {
		var v struct {
			Tables        []TableInfo      `json:"tables"`
			CoreCounts    map[string]int64 `json:"coreCounts"`
			AOTables      int              `json:"aoTables"`
			UnknownTables int              `json:"unknownTables"`
		}
		_ = json.Unmarshal(summary, &v)
		x.Tables = v.Tables
		x.CoreCounts = v.CoreCounts
		x.AOTables = v.AOTables
		x.UnknownTables = v.UnknownTables
		d.LatestDiscovery = &x
	}
	return d, nil
}

func tableExists(tables []TableInfo, name string) bool {
	for _, t := range tables {
		if strings.EqualFold(t.Name, name) {
			return true
		}
	}
	return false
}
func knownTables() map[string]bool {
	names := []string{"CONTENT", "BODYCONTENT", "SPACES", "CONTENT_PERM", "CONTENT_PERM_SET", "CONTENTPROPERTIES", "CONTENT_LABEL", "LABEL", "SPACEPERMISSIONS", "PAGETEMPLATES", "CONFANCESTORS", "CWD_USER", "CWD_GROUP", "CWD_MEMBERSHIP", "CWD_DIRECTORY", "CWD_USER_ATTRIBUTE", "CWD_GROUP_ATTRIBUTE", "USER_MAPPING", "LIKES", "NOTIFICATIONS", "FOLLOW_CONNECTIONS", "BANDANA", "CONFVERSION", "ATTACHMENTS"}
	m := map[string]bool{}
	for _, n := range names {
		m[n] = true
	}
	return m
}
