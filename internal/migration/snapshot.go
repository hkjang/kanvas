package migration

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type legacySource struct {
	db          *sql.DB
	columnCache map[string]map[string]bool
}

func (s *legacySource) close() { _ = s.db.Close() }
func (s *legacySource) tableExists(ctx context.Context, table string) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND UPPER(TABLE_NAME)=UPPER(?)`, table).Scan(&n)
	return n > 0, err
}
func (s *legacySource) tableColumns(ctx context.Context, table string) (map[string]bool, error) {
	key := strings.ToUpper(table)
	if v := s.columnCache[key]; v != nil {
		return v, nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT UPPER(COLUMN_NAME) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND UPPER(TABLE_NAME)=UPPER(?)`, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out[name] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	s.columnCache[key] = out
	return out, nil
}
func (s *legacySource) countTable(ctx context.Context, table string) (int64, error) {
	ok, err := s.tableExists(ctx, table)
	if err != nil || !ok {
		return 0, err
	}
	var n int64
	err = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM `"+table+"`").Scan(&n)
	return n, err
}
func (s *legacySource) countSnapshotItems(ctx context.Context, o SnapshotOptions) (int64, error) {
	var total int64
	tables := []struct {
		enabled bool
		name    string
	}{{o.IncludeUsers, "CWD_USER"}, {o.IncludeGroups, "CWD_GROUP"}, {o.IncludeGroups, "CWD_MEMBERSHIP"}, {o.IncludeSpaces, "SPACES"}}
	if o.IncludePermissions {
		for _, table := range []string{"SPACEPERMISSIONS", "CONTENT_PERM"} {
			n, err := s.countTable(ctx, table)
			if err != nil {
				return 0, err
			}
			total += n
		}
	}
	for _, x := range tables {
		if x.enabled {
			n, err := s.countTable(ctx, x.name)
			if err != nil {
				return 0, err
			}
			total += n
		}
	}
	if o.IncludePages || o.IncludeComments || o.IncludeAttachmentMetadata {
		kinds := []string{}
		if o.IncludePages {
			kinds = append(kinds, "'PAGE'", "'BLOGPOST'")
		}
		if o.IncludeComments {
			kinds = append(kinds, "'COMMENT'")
		}
		if o.IncludeAttachmentMetadata {
			kinds = append(kinds, "'ATTACHMENT'")
		}
		ok, err := s.tableExists(ctx, "CONTENT")
		if err != nil {
			return 0, err
		}
		if ok {
			var n int64
			err = s.db.QueryRowContext(ctx, `SELECT count(*) FROM CONTENT WHERE UPPER(CONTENTTYPE) IN (`+strings.Join(kinds, ",")+`)`).Scan(&n)
			if err != nil {
				return 0, err
			}
			total += n
		}
	}
	return total, nil
}

type snapshotRunner struct {
	service        *Service
	source         *legacySource
	jobID          uuid.UUID
	actor          uuid.UUID
	options        SnapshotOptions
	usersByLegacy  map[string]uuid.UUID
	usersByName    map[string]uuid.UUID
	groupsByLegacy map[string]uuid.UUID
	groupsByName   map[string]uuid.UUID
	spacesByLegacy map[string]uuid.UUID
	logicalPage    map[string]string
	pageTargets    map[string]uuid.UUID
}

type legacyContent struct {
	ID, Kind, Title, SpaceID, ParentID, PrevID, Status, Creator, Created, Modifier, Modified, Body string
	Version                                                                                        int
}

func (s *Service) runSnapshot(ctx context.Context, dsn string, jobID, actor uuid.UUID, o SnapshotOptions) {
	_, _ = s.Store.Pool.Exec(context.Background(), `UPDATE migration_jobs SET status='RUNNING',started_at=now() WHERE id=$1`, jobID)
	source, err := openLegacy(ctx, dsn)
	if err != nil {
		s.failJob(jobID, err)
		return
	}
	defer source.close()
	r := &snapshotRunner{service: s, source: source, jobID: jobID, actor: actor, options: o, usersByLegacy: map[string]uuid.UUID{}, usersByName: map[string]uuid.UUID{}, groupsByLegacy: map[string]uuid.UUID{}, groupsByName: map[string]uuid.UUID{}, spacesByLegacy: map[string]uuid.UUID{}, logicalPage: map[string]string{}, pageTargets: map[string]uuid.UUID{}}
	steps := []struct {
		name    string
		enabled bool
		run     func(context.Context) error
	}{{"USERS", o.IncludeUsers, r.importUsers}, {"GROUPS", o.IncludeGroups, r.importGroups}, {"MEMBERSHIPS", o.IncludeGroups, r.importMemberships}, {"SPACES", o.IncludeSpaces, r.importSpaces}, {"CONTENT", o.IncludePages || o.IncludeComments || o.IncludeAttachmentMetadata, r.importContent}, {"PERMISSIONS", o.IncludePermissions, r.importPermissions}}
	for _, step := range steps {
		if !step.enabled {
			continue
		}
		if ctx.Err() != nil {
			s.cancelledJob(jobID)
			return
		}
		_, _ = s.Store.Pool.Exec(context.Background(), `UPDATE migration_jobs SET current_entity=$2 WHERE id=$1`, jobID, step.name)
		if err = step.run(ctx); err != nil {
			if errors.Is(err, context.Canceled) {
				s.cancelledJob(jobID)
			} else {
				s.failJob(jobID, err)
			}
			return
		}
	}
	if err = r.reconcile(context.Background()); err != nil {
		s.failJob(jobID, err)
		return
	}
	var processed, failed int64
	_ = s.Store.Pool.QueryRow(context.Background(), `SELECT count(*) FILTER(WHERE status IN ('COMPLETE','FAILED')),count(*) FILTER(WHERE status='FAILED') FROM migration_items WHERE job_id=$1`, jobID).Scan(&processed, &failed)
	status := "COMPLETE"
	if failed > 0 {
		status = "COMPLETED_WITH_ERRORS"
	}
	_, _ = s.Store.Pool.Exec(context.Background(), `UPDATE migration_jobs SET status=$2,processed_items=$3,failed_items=$4,current_entity='',finished_at=now(),checkpoint=checkpoint||jsonb_build_object('completed',true) WHERE id=$1`, jobID, status, processed, failed)
	_, _ = s.Store.Pool.Exec(context.Background(), `UPDATE migration_state SET updated_at=now(),details=details||jsonb_build_object('lastSnapshotJobId',$1::uuid,'lastSnapshotStatus',$2::text) WHERE id=true`, jobID, status)
	s.Store.Audit(context.Background(), &actor, "MIGRATION_SNAPSHOT_FINISH", "MIGRATION_JOB", jobID.String(), "", "", map[string]any{"status": status, "failedItems": failed})
}

func (s *Service) failJob(jobID uuid.UUID, err error) {
	_, _ = s.Store.Pool.Exec(context.Background(), `UPDATE migration_jobs SET status='FAILED',error=$2,finished_at=now(),current_entity='' WHERE id=$1`, jobID, truncate(err.Error(), 4000))
	_, _ = s.Store.Pool.Exec(context.Background(), `UPDATE migration_state SET phase='ERROR',updated_at=now(),details=details||jsonb_build_object('lastError',$1::text) WHERE id=true`, truncate(err.Error(), 4000))
}
func (s *Service) cancelledJob(jobID uuid.UUID) {
	_, _ = s.Store.Pool.Exec(context.Background(), `UPDATE migration_jobs SET status='CANCELLED',cancel_requested=true,finished_at=now(),current_entity='' WHERE id=$1`, jobID)
	_, _ = s.Store.Pool.Exec(context.Background(), `UPDATE migration_state SET phase='DISCOVERY',updated_at=now() WHERE id=true AND phase='SNAPSHOT'`)
}

func (r *snapshotRunner) apply(ctx context.Context, entity, legacyID string, targetID uuid.UUID, details any, fn func() error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	detailJSON, _ := json.Marshal(details)
	itemID := deterministicID("MIGRATION_ITEM", r.jobID.String()+":"+entity+":"+legacyID)
	var previousStatus string
	previousErr := r.service.Store.Pool.QueryRow(ctx, `SELECT status FROM migration_items WHERE job_id=$1 AND entity_type=$2 AND legacy_id=$3`, r.jobID, entity, legacyID).Scan(&previousStatus)
	if previousErr == nil && previousStatus == "COMPLETE" {
		return nil
	}
	if previousErr != nil && !errors.Is(previousErr, pgx.ErrNoRows) {
		return previousErr
	}
	_, err := r.service.Store.Pool.Exec(ctx, `INSERT INTO migration_items(id,job_id,entity_type,legacy_id,target_id,status,details,started_at) VALUES($1,$2,$3,$4,$5,'RUNNING',$6,now()) ON CONFLICT(job_id,entity_type,legacy_id) DO UPDATE SET target_id=excluded.target_id,status='RUNNING',error='',details=excluded.details,started_at=now(),finished_at=NULL`, itemID, r.jobID, entity, legacyID, targetID, detailJSON)
	if err != nil {
		return err
	}
	err = fn()
	status := "COMPLETE"
	errorText := ""
	failedDelta := 0
	if err != nil {
		status = "FAILED"
		errorText = truncate(err.Error(), 4000)
		failedDelta = 1
	}
	if err == nil {
		_, err = r.service.Store.Pool.Exec(ctx, `INSERT INTO migration_mapping(id,legacy_system,entity_type,legacy_id,target_id,metadata) VALUES($1,'CONFLUENCE',$2,$3,$4,$5) ON CONFLICT(legacy_system,entity_type,legacy_id) DO UPDATE SET target_id=excluded.target_id,metadata=excluded.metadata,updated_at=now()`, deterministicID("MAPPING", entity+":"+legacyID), entity, legacyID, targetID, detailJSON)
		if err != nil {
			status = "FAILED"
			errorText = truncate(err.Error(), 4000)
			failedDelta = 1
		}
	}
	_, updateErr := r.service.Store.Pool.Exec(ctx, `UPDATE migration_items SET status=$2,error=$3,retry_count=CASE WHEN $2='FAILED' THEN retry_count+1 ELSE retry_count END,finished_at=now() WHERE id=$1`, itemID, status, errorText)
	if updateErr != nil {
		return updateErr
	}
	processedDelta := 1
	if previousErr == nil && (previousStatus == "COMPLETE" || previousStatus == "FAILED") {
		processedDelta = 0
	}
	if previousStatus == "FAILED" {
		failedDelta--
	}
	checkpoint, _ := json.Marshal(map[string]any{"entity": entity, "legacyId": legacyID})
	_, updateErr = r.service.Store.Pool.Exec(ctx, `UPDATE migration_jobs SET processed_items=processed_items+$2,failed_items=GREATEST(failed_items+$3,0),checkpoint=$4 WHERE id=$1`, r.jobID, processedDelta, failedDelta, checkpoint)
	if updateErr != nil {
		return updateErr
	}
	return nil
}

func (r *snapshotRunner) importUsers(ctx context.Context) error {
	ok, err := r.source.tableExists(ctx, "CWD_USER")
	if err != nil || !ok {
		return err
	}
	cols, err := r.source.tableColumns(ctx, "CWD_USER")
	if err != nil {
		return err
	}
	if !cols["ID"] {
		return errors.New("CWD_USER.ID is required")
	}
	query := `SELECT ` + strExpr("u", cols, "", "ID") + `,` + strExpr("u", cols, "", "USER_NAME", "USERNAME", "LOWER_USER_NAME") + `,` + strExpr("u", cols, "", "DISPLAY_NAME", "USER_NAME") + `,` + strExpr("u", cols, "", "EMAIL_ADDRESS", "EMAIL") + `,` + strExpr("u", cols, "T", "ACTIVE") + ` FROM CWD_USER u ORDER BY u.ID`
	rows, err := r.source.db.QueryContext(ctx, query)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var legacyID, username, display, email, active string
		if err := rows.Scan(&legacyID, &username, &display, &email, &active); err != nil {
			return err
		}
		username = strings.TrimSpace(username)
		if username == "" {
			username = "legacy_" + legacyID
		}
		target := deterministicID("USER", legacyID)
		var existing uuid.UUID
		e := r.service.Store.Pool.QueryRow(ctx, `SELECT id FROM users WHERE lower(username)=lower($1)`, username).Scan(&existing)
		if e == nil {
			target = existing
		} else if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		err = r.apply(ctx, "USER", legacyID, target, map[string]any{"username": username}, func() error {
			status := "ACTIVE"
			if active == "0" || strings.EqualFold(active, "F") || strings.EqualFold(active, "false") {
				status = "DISABLED"
			}
			_, err := r.service.Store.Pool.Exec(ctx, `INSERT INTO users(id,username,display_name,email,role,status,identity_provider,legacy_system,legacy_id) VALUES($1,$2,$3,$4,'USER',$5,'MIGRATED','CONFLUENCE',$6) ON CONFLICT(id) DO UPDATE SET display_name=excluded.display_name,email=excluded.email,status=excluded.status,legacy_system='CONFLUENCE',legacy_id=excluded.legacy_id,updated_at=now()`, target, username, firstNonEmpty(display, username), email, status, legacyID)
			return err
		})
		if err != nil {
			return err
		}
		r.usersByLegacy[legacyID] = target
		r.usersByName[strings.ToLower(username)] = target
	}
	return rows.Err()
}

func (r *snapshotRunner) importGroups(ctx context.Context) error {
	ok, err := r.source.tableExists(ctx, "CWD_GROUP")
	if err != nil || !ok {
		return err
	}
	cols, err := r.source.tableColumns(ctx, "CWD_GROUP")
	if err != nil {
		return err
	}
	if !cols["ID"] {
		return errors.New("CWD_GROUP.ID is required")
	}
	query := `SELECT ` + strExpr("g", cols, "", "ID") + `,` + strExpr("g", cols, "", "GROUP_NAME", "NAME") + `,` + strExpr("g", cols, "", "DESCRIPTION") + ` FROM CWD_GROUP g ORDER BY g.ID`
	rows, err := r.source.db.QueryContext(ctx, query)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var legacyID, name, description string
		if err := rows.Scan(&legacyID, &name, &description); err != nil {
			return err
		}
		if name == "" {
			name = "legacy-group-" + legacyID
		}
		target := deterministicID("GROUP", legacyID)
		var existing uuid.UUID
		e := r.service.Store.Pool.QueryRow(ctx, `SELECT id FROM groups WHERE lower(name)=lower($1)`, name).Scan(&existing)
		if e == nil {
			target = existing
		} else if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		if err := r.apply(ctx, "GROUP", legacyID, target, map[string]any{"name": name}, func() error {
			_, err := r.service.Store.Pool.Exec(ctx, `INSERT INTO groups(id,name,description,legacy_system,legacy_id) VALUES($1,$2,$3,'CONFLUENCE',$4) ON CONFLICT(id) DO UPDATE SET name=excluded.name,description=excluded.description,legacy_system='CONFLUENCE',legacy_id=excluded.legacy_id`, target, name, description, legacyID)
			return err
		}); err != nil {
			return err
		}
		r.groupsByLegacy[legacyID] = target
		r.groupsByName[strings.ToLower(name)] = target
	}
	return rows.Err()
}

func (r *snapshotRunner) importMemberships(ctx context.Context) error {
	ok, err := r.source.tableExists(ctx, "CWD_MEMBERSHIP")
	if err != nil || !ok {
		return err
	}
	cols, err := r.source.tableColumns(ctx, "CWD_MEMBERSHIP")
	if err != nil {
		return err
	}
	if !cols["PARENT_ID"] || !cols["CHILD_USER_ID"] {
		return nil
	}
	query := `SELECT ` + strExpr("m", cols, "", "ID") + `,` + strExpr("m", cols, "", "PARENT_ID") + `,` + strExpr("m", cols, "", "CHILD_USER_ID") + ` FROM CWD_MEMBERSHIP m WHERE m.CHILD_USER_ID IS NOT NULL ORDER BY m.PARENT_ID`
	rows, err := r.source.db.QueryContext(ctx, query)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, groupLegacy, userLegacy string
		if err := rows.Scan(&id, &groupLegacy, &userLegacy); err != nil {
			return err
		}
		if id == "" {
			id = groupLegacy + ":" + userLegacy
		}
		target := deterministicID("MEMBERSHIP", id)
		if err := r.apply(ctx, "GROUP_MEMBER", id, target, map[string]any{"groupLegacyId": groupLegacy, "userLegacyId": userLegacy}, func() error {
			groupID, gok := r.groupsByLegacy[groupLegacy]
			userID, uok := r.usersByLegacy[userLegacy]
			if !gok || !uok {
				return errors.New("membership references a missing user or group")
			}
			_, err := r.service.Store.Pool.Exec(ctx, `INSERT INTO group_members(group_id,user_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, groupID, userID)
			return err
		}); err != nil {
			return err
		}
	}
	return rows.Err()
}

func (r *snapshotRunner) importSpaces(ctx context.Context) error {
	ok, err := r.source.tableExists(ctx, "SPACES")
	if err != nil || !ok {
		return err
	}
	cols, err := r.source.tableColumns(ctx, "SPACES")
	if err != nil {
		return err
	}
	if !cols["SPACEID"] {
		return errors.New("SPACES.SPACEID is required")
	}
	query := `SELECT ` + strExpr("s", cols, "", "SPACEID") + `,` + strExpr("s", cols, "", "SPACEKEY") + `,` + strExpr("s", cols, "", "SPACENAME") + ` FROM SPACES s ORDER BY s.SPACEID`
	rows, err := r.source.db.QueryContext(ctx, query)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var legacyID, key, name string
		if err := rows.Scan(&legacyID, &key, &name); err != nil {
			return err
		}
		if key == "" {
			key = "LEGACY" + legacyID
		}
		if name == "" {
			name = key
		}
		target := deterministicID("SPACE", legacyID)
		var existing uuid.UUID
		e := r.service.Store.Pool.QueryRow(ctx, `SELECT id FROM spaces WHERE upper(space_key)=upper($1)`, key).Scan(&existing)
		if e == nil {
			target = existing
		} else if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		if err := r.apply(ctx, "SPACE", legacyID, target, map[string]any{"key": key}, func() error {
			_, err := r.service.Store.Pool.Exec(ctx, `INSERT INTO spaces(id,space_key,name,description,status,legacy_system,legacy_id) VALUES($1,$2,$3,'','ACTIVE','CONFLUENCE',$4) ON CONFLICT(id) DO UPDATE SET name=excluded.name,status='ACTIVE',legacy_system='CONFLUENCE',legacy_id=excluded.legacy_id,updated_at=now()`, target, strings.ToUpper(key), name, legacyID)
			return err
		}); err != nil {
			return err
		}
		r.spacesByLegacy[legacyID] = target
	}
	return rows.Err()
}

func (r *snapshotRunner) importContent(ctx context.Context) error {
	records, err := r.loadContent(ctx)
	if err != nil {
		return err
	}
	r.resolveLogicalPages(records)
	var pages, comments, attachments []*legacyContent
	for _, c := range records {
		switch strings.ToUpper(c.Kind) {
		case "PAGE", "BLOGPOST":
			pages = append(pages, c)
		case "COMMENT":
			comments = append(comments, c)
		case "ATTACHMENT":
			attachments = append(attachments, c)
		}
	}
	if r.options.IncludePages {
		if err = r.importPages(ctx, pages); err != nil {
			return err
		}
	}
	if r.options.IncludeComments {
		if err = r.importComments(ctx, comments); err != nil {
			return err
		}
	}
	if r.options.IncludeAttachmentMetadata {
		if err = r.importAttachments(ctx, attachments); err != nil {
			return err
		}
	}
	return nil
}

func (r *snapshotRunner) loadContent(ctx context.Context) ([]*legacyContent, error) {
	ok, err := r.source.tableExists(ctx, "CONTENT")
	if err != nil || !ok {
		return nil, err
	}
	cols, err := r.source.tableColumns(ctx, "CONTENT")
	if err != nil {
		return nil, err
	}
	if !cols["CONTENTID"] || !cols["CONTENTTYPE"] {
		return nil, errors.New("CONTENT.CONTENTID and CONTENT.CONTENTTYPE are required")
	}
	bodyExpr := "''"
	bodyExists, _ := r.source.tableExists(ctx, "BODYCONTENT")
	if bodyExists {
		bodyCols, e := r.source.tableColumns(ctx, "BODYCONTENT")
		if e != nil {
			return nil, e
		}
		if bodyCols["CONTENTID"] && bodyCols["BODY"] {
			order := ""
			if bodyCols["BODYTYPEID"] {
				order = " ORDER BY bc.BODYTYPEID DESC"
			}
			bodyExpr = `COALESCE((SELECT CAST(bc.BODY AS CHAR) FROM BODYCONTENT bc WHERE bc.CONTENTID=c.CONTENTID` + order + ` LIMIT 1),'')`
		}
	}
	query := `SELECT ` + strExpr("c", cols, "", "CONTENTID") + `,` + strExpr("c", cols, "", "CONTENTTYPE") + `,` + strExpr("c", cols, "", "TITLE") + `,` + strExpr("c", cols, "", "SPACEID") + `,` + strExpr("c", cols, "", "PARENTID") + `,` + strExpr("c", cols, "", "PREVVER") + `,` + strExpr("c", cols, "CURRENT", "CONTENT_STATUS", "CONTENTSTATUS") + `,` + intExpr("c", cols, 1, "VERSION") + `,` + strExpr("c", cols, "", "CREATOR") + `,` + dateExpr("c", cols, "CREATIONDATE") + `,` + strExpr("c", cols, "", "LASTMODIFIER") + `,` + dateExpr("c", cols, "LASTMODDATE") + `,` + bodyExpr + ` FROM CONTENT c WHERE UPPER(c.CONTENTTYPE) IN ('PAGE','BLOGPOST','COMMENT','ATTACHMENT') ORDER BY c.CONTENTID`
	rows, err := r.source.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*legacyContent
	for rows.Next() {
		v := &legacyContent{}
		if err := rows.Scan(&v.ID, &v.Kind, &v.Title, &v.SpaceID, &v.ParentID, &v.PrevID, &v.Status, &v.Version, &v.Creator, &v.Created, &v.Modifier, &v.Modified, &v.Body); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *snapshotRunner) resolveLogicalPages(records []*legacyContent) {
	byID := map[string]*legacyContent{}
	children := map[string][]*legacyContent{}
	for _, c := range records {
		if c.Kind == "PAGE" || c.Kind == "BLOGPOST" {
			byID[c.ID] = c
			if c.PrevID != "" {
				children[c.PrevID] = append(children[c.PrevID], c)
			}
		}
	}
	for id := range byID {
		current := byID[id]
		seen := map[string]bool{}
		for len(children[current.ID]) > 0 && !seen[current.ID] {
			seen[current.ID] = true
			candidates := children[current.ID]
			sort.Slice(candidates, func(i, j int) bool { return candidates[i].Version > candidates[j].Version })
			current = candidates[0]
		}
		r.logicalPage[id] = current.ID
	}
	for id, logical := range r.logicalPage {
		r.pageTargets[id] = deterministicID("PAGE", logical)
	}
}

func (r *snapshotRunner) importPages(ctx context.Context, records []*legacyContent) error {
	groups := map[string][]*legacyContent{}
	for _, c := range records {
		logical := r.logicalPage[c.ID]
		groups[logical] = append(groups[logical], c)
	}
	logicalIDs := make([]string, 0, len(groups))
	for id := range groups {
		logicalIDs = append(logicalIDs, id)
	}
	sort.Strings(logicalIDs)
	for _, logical := range logicalIDs {
		versions := groups[logical]
		sort.Slice(versions, func(i, j int) bool {
			if versions[i].Version == versions[j].Version {
				return versions[i].ID < versions[j].ID
			}
			return versions[i].Version < versions[j].Version
		})
		used := map[int]bool{}
		for i, c := range versions {
			if c.Version < 1 || used[c.Version] {
				c.Version = i + 1
			}
			used[c.Version] = true
		}
		latest := versions[len(versions)-1]
		for _, c := range versions {
			if strings.EqualFold(c.Status, "current") {
				latest = c
			}
		}
		pageID := deterministicID("PAGE", logical)
		spaceID, err := r.resolveSpace(ctx, latest.SpaceID)
		if err != nil {
			return err
		}
		for _, c := range append([]*legacyContent{latest}, withoutContent(versions, latest.ID)...) {
			entity := "PAGE_VERSION"
			if c.ID == latest.ID {
				entity = "PAGE"
			}
			conv := ConvertStorage(c.Body)
			versionID := deterministicID("PAGE_VERSION", c.ID)
			target := versionID
			if entity == "PAGE" {
				target = pageID
			}
			created := parseLegacyTime(c.Created)
			modified := parseLegacyTime(firstNonEmpty(c.Modified, c.Created))
			creator := r.userFor(c.Creator)
			modifier := r.userFor(firstNonEmpty(c.Modifier, c.Creator))
			expectedHash := fmt.Sprintf("%x", sha256.Sum256([]byte(conv.Text)))
			details := map[string]any{"logicalPageId": logical, "version": c.Version, "contentHash": expectedHash, "macros": conv.Macros, "warnings": conv.Warnings}
			if err := r.apply(ctx, entity, c.ID, target, details, func() error {
				if entity == "PAGE" {
					_, err := r.service.Store.Pool.Exec(ctx, `INSERT INTO pages(id,space_id,parent_id,title,status,current_version,legacy_system,legacy_id,created_by,updated_by,created_at,updated_at) VALUES($1,$2,NULL,$3,'CURRENT',$4,'CONFLUENCE',$5,$6,$7,$8,$9) ON CONFLICT(id) DO UPDATE SET space_id=excluded.space_id,title=excluded.title,status='CURRENT',current_version=excluded.current_version,legacy_system='CONFLUENCE',legacy_id=excluded.legacy_id,updated_by=excluded.updated_by,updated_at=excluded.updated_at`, pageID, spaceID, firstNonEmpty(latest.Title, "Untitled"), latest.Version, logical, creator, modifier, created, modified)
					if err != nil {
						return err
					}
				}
				_, err := r.service.Store.Pool.Exec(ctx, `INSERT INTO page_versions(id,page_id,version,title,legacy_system,legacy_id,legacy_storage,canonical_document,editor_document,rendered_text,change_message,created_by,created_at,content_hash) VALUES($1,$2,$3,$4,'CONFLUENCE',$5,$6,$7,$8,$9,'Migrated from Confluence',$10,$11,$12) ON CONFLICT(page_id,version) DO UPDATE SET title=excluded.title,legacy_system='CONFLUENCE',legacy_id=excluded.legacy_id,legacy_storage=excluded.legacy_storage,canonical_document=excluded.canonical_document,editor_document=excluded.editor_document,rendered_text=excluded.rendered_text,content_hash=excluded.content_hash`, versionID, pageID, c.Version, firstNonEmpty(c.Title, latest.Title, "Untitled"), c.ID, c.Body, conv.Canonical, conv.Editor, conv.Text, creator, created, expectedHash)
				if err != nil {
					return err
				}
				if err = r.recordPageExtensions(ctx, pageID, c.Version, c.ID, conv); err != nil {
					return err
				}
				_, err = r.service.Store.Pool.Exec(ctx, `INSERT INTO migration_mapping(id,legacy_system,entity_type,legacy_id,target_id,metadata) VALUES($1,'CONFLUENCE','PAGE',$2,$3,jsonb_build_object('version',$4::integer)) ON CONFLICT(legacy_system,entity_type,legacy_id) DO UPDATE SET target_id=excluded.target_id,metadata=excluded.metadata,updated_at=now()`, deterministicID("MAPPING", "PAGE:"+c.ID), c.ID, pageID, c.Version)
				return err
			}); err != nil {
				return err
			}
			_, _ = r.service.Store.Pool.Exec(ctx, `UPDATE migration_items SET source_hash=$4,target_hash=coalesce((SELECT content_hash FROM page_versions WHERE id=$5),'') WHERE job_id=$1 AND entity_type=$2 AND legacy_id=$3`, r.jobID, entity, c.ID, expectedHash, versionID)
		}
	}
	for _, logical := range logicalIDs {
		latest := latestContent(groups[logical])
		if latest.ParentID == "" {
			continue
		}
		parentLogical := r.logicalPage[latest.ParentID]
		if parentLogical == "" {
			parentLogical = latest.ParentID
		}
		pageID := deterministicID("PAGE", logical)
		parentID := deterministicID("PAGE", parentLogical)
		cmd, err := r.service.Store.Pool.Exec(ctx, `UPDATE pages SET parent_id=$2 WHERE id=$1 AND EXISTS(SELECT 1 FROM pages WHERE id=$2)`, pageID, parentID)
		if err != nil {
			return err
		}
		if cmd.RowsAffected() == 0 {
			r.unsupported(ctx, pageID, latest.ID, "ORPHAN_PAGE", latest.ParentID, "Parent page was not found")
		}
	}
	return nil
}

func (r *snapshotRunner) recordPageExtensions(ctx context.Context, pageID uuid.UUID, version int, legacyID string, conv ConversionResult) error {
	for _, m := range conv.Macros {
		_, err := r.service.Store.Pool.Exec(ctx, `INSERT INTO page_macros(page_id,page_version,macro_name,supported,occurrence_count) VALUES($1,$2,$3,$4,$5) ON CONFLICT(page_id,page_version,macro_name) DO UPDATE SET supported=excluded.supported,occurrence_count=excluded.occurrence_count`, pageID, version, m.Name, m.Supported, max(m.Occurrences, 1))
		if err != nil {
			return err
		}
		if !m.Supported {
			r.unsupportedCount(ctx, pageID, legacyID, "UNKNOWN_MACRO", m.Name, "Unsupported Confluence macro", int64(max(m.Occurrences, 1)))
		}
	}
	for _, warning := range conv.Warnings {
		r.unsupported(ctx, pageID, legacyID, "INVALID_XHTML", "parser", warning)
	}
	if version > 0 {
		var current int
		if err := r.service.Store.Pool.QueryRow(ctx, `SELECT current_version FROM pages WHERE id=$1`, pageID).Scan(&current); err == nil && current == version {
			_, _ = r.service.Store.Pool.Exec(ctx, `DELETE FROM page_links WHERE source_page_id=$1`, pageID)
			for _, link := range conv.Links {
				_, err := r.service.Store.Pool.Exec(ctx, `INSERT INTO page_links(id,source_page_id,link_type,target,legacy_target_id) VALUES($1,$2,$3,$4,$5) ON CONFLICT(source_page_id,link_type,target) DO NOTHING`, deterministicID("PAGE_LINK", pageID.String()+":"+link.Type+":"+link.Target), pageID, link.Type, link.Target, map[bool]string{true: link.Target, false: ""}[link.Type == "PAGE"])
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (r *snapshotRunner) importComments(ctx context.Context, records []*legacyContent) error {
	commentTargets := map[string]uuid.UUID{}
	for _, c := range records {
		commentTargets[c.ID] = deterministicID("COMMENT", c.ID)
	}
	for _, c := range records {
		pageLogical := r.logicalPage[c.ParentID]
		if pageLogical == "" {
			pageLogical = c.ParentID
		}
		pageID := deterministicID("PAGE", pageLogical)
		commentID := commentTargets[c.ID]
		conv := ConvertStorage(c.Body)
		created := parseLegacyTime(c.Created)
		creator := r.userFor(c.Creator)
		if err := r.apply(ctx, "COMMENT", c.ID, commentID, map[string]any{"pageLegacyId": pageLogical}, func() error {
			var exists bool
			if err := r.service.Store.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pages WHERE id=$1)`, pageID).Scan(&exists); err != nil {
				return err
			}
			if !exists {
				return errors.New("comment parent page was not found")
			}
			_, err := r.service.Store.Pool.Exec(ctx, `INSERT INTO comments(id,page_id,parent_id,body,created_by,created_at,updated_at,legacy_system,legacy_id) VALUES($1,$2,NULL,$3,$4,$5,$5,'CONFLUENCE',$6) ON CONFLICT(id) DO UPDATE SET page_id=excluded.page_id,body=excluded.body,updated_at=excluded.updated_at`, commentID, pageID, conv.Text, creator, created, c.ID)
			return err
		}); err != nil {
			return err
		}
	}
	return nil
}

func (r *snapshotRunner) importAttachments(ctx context.Context, records []*legacyContent) error {
	for _, c := range records {
		pageLogical := r.logicalPage[c.ParentID]
		if pageLogical == "" {
			pageLogical = c.ParentID
		}
		pageID := deterministicID("PAGE", pageLogical)
		attachmentID := deterministicID("ATTACHMENT", c.ID)
		created := parseLegacyTime(c.Created)
		creator := r.userFor(c.Creator)
		if err := r.apply(ctx, "ATTACHMENT", c.ID, attachmentID, map[string]any{"pageLegacyId": pageLogical, "binaryStatus": "PENDING"}, func() error {
			var exists bool
			if err := r.service.Store.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pages WHERE id=$1)`, pageID).Scan(&exists); err != nil {
				return err
			}
			if !exists {
				return errors.New("attachment parent page was not found")
			}
			_, err := r.service.Store.Pool.Exec(ctx, `INSERT INTO attachments(id,page_id,filename,media_type,size,sha256,storage_key,version,legacy_system,legacy_id,created_by,created_at) VALUES($1,$2,$3,'application/octet-stream',0,'',$4,$5,'CONFLUENCE',$6,$7,$8) ON CONFLICT(id) DO UPDATE SET page_id=excluded.page_id,filename=excluded.filename,version=excluded.version`, attachmentID, pageID, firstNonEmpty(c.Title, "attachment-"+c.ID), "legacy:"+c.ID, max(c.Version, 1), c.ID, creator, created)
			return err
		}); err != nil {
			return err
		}
	}
	return nil
}

func (r *snapshotRunner) importPermissions(ctx context.Context) error {
	if err := r.importSpacePermissions(ctx); err != nil {
		return err
	}
	return r.importPagePermissions(ctx)
}

func (r *snapshotRunner) importSpacePermissions(ctx context.Context) error {
	ok, err := r.source.tableExists(ctx, "SPACEPERMISSIONS")
	if err != nil || !ok {
		return err
	}
	cols, err := r.source.tableColumns(ctx, "SPACEPERMISSIONS")
	if err != nil {
		return err
	}
	idColumn := firstColumn(cols, "PERMID", "ID")
	spaceColumn := firstColumn(cols, "SPACEID", "SPACE_ID")
	typeColumn := firstColumn(cols, "PERMTYPE", "PERMISSION_TYPE", "TYPE")
	if idColumn == "" || spaceColumn == "" || typeColumn == "" {
		return errors.New("SPACEPERMISSIONS requires an id, space id, and permission type column")
	}
	query := `SELECT ` + strExpr("sp", cols, "", idColumn) + `,` + strExpr("sp", cols, "", spaceColumn) + `,` + strExpr("sp", cols, "", typeColumn) + `,` + strExpr("sp", cols, "", "PERMUSERNAME", "USERNAME", "USER_NAME") + `,` + strExpr("sp", cols, "", "PERMGROUPNAME", "GROUPNAME", "GROUP_NAME") + ` FROM SPACEPERMISSIONS sp ORDER BY sp.` + "`" + idColumn + "`"
	rows, err := r.source.db.QueryContext(ctx, query)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var legacyID, spaceLegacy, rawPermission, username, groupName string
		if err := rows.Scan(&legacyID, &spaceLegacy, &rawPermission, &username, &groupName); err != nil {
			return err
		}
		permission, supported := canonicalSpacePermission(rawPermission)
		target := deterministicID("SPACE_PERMISSION", legacyID)
		details := map[string]any{"spaceLegacyId": spaceLegacy, "sourcePermission": rawPermission, "permission": permission, "username": username, "groupName": groupName}
		if err := r.apply(ctx, "SPACE_PERMISSION", legacyID, target, details, func() error {
			if !supported {
				return fmt.Errorf("unsupported Confluence space permission %q", rawPermission)
			}
			spaceID, err := r.permissionSpace(ctx, spaceLegacy)
			if err != nil {
				return err
			}
			subjectType, subjectID, err := r.permissionSubject(ctx, username, groupName)
			if err != nil {
				return err
			}
			_, err = r.service.Store.Pool.Exec(ctx, `INSERT INTO space_permissions(id,space_id,permission,subject_type,subject_id,legacy_system,legacy_id) VALUES($1,$2,$3,$4,$5,'CONFLUENCE',$6) ON CONFLICT(id) DO UPDATE SET space_id=excluded.space_id,permission=excluded.permission,subject_type=excluded.subject_type,subject_id=excluded.subject_id,legacy_system='CONFLUENCE',legacy_id=excluded.legacy_id`, target, spaceID, permission, subjectType, subjectID, legacyID)
			return err
		}); err != nil {
			return err
		}
	}
	return rows.Err()
}

func (r *snapshotRunner) importPagePermissions(ctx context.Context) error {
	permExists, err := r.source.tableExists(ctx, "CONTENT_PERM")
	if err != nil || !permExists {
		return err
	}
	setExists, err := r.source.tableExists(ctx, "CONTENT_PERM_SET")
	if err != nil || !setExists {
		if err != nil {
			return err
		}
		return errors.New("CONTENT_PERM exists but CONTENT_PERM_SET is missing")
	}
	permCols, err := r.source.tableColumns(ctx, "CONTENT_PERM")
	if err != nil {
		return err
	}
	setCols, err := r.source.tableColumns(ctx, "CONTENT_PERM_SET")
	if err != nil {
		return err
	}
	idColumn := firstColumn(permCols, "ID", "PERMID")
	setFKColumn := firstColumn(permCols, "CPS_ID", "CONTENT_PERM_SET_ID", "PERM_SET_ID")
	setIDColumn := firstColumn(setCols, "ID", "CPS_ID")
	contentColumn := firstColumn(setCols, "CONTENT_ID", "CONTENTID")
	typeColumn := firstColumn(setCols, "CONT_PERM_TYPE", "PERMTYPE", "PERMISSION_TYPE", "TYPE")
	if idColumn == "" || setFKColumn == "" || setIDColumn == "" || contentColumn == "" || typeColumn == "" {
		return errors.New("CONTENT_PERM schema is missing required id, set, content, or type columns")
	}
	query := `SELECT ` + strExpr("cp", permCols, "", idColumn) + `,` + strExpr("cps", setCols, "", contentColumn) + `,` + strExpr("cps", setCols, "", typeColumn) + `,` + strExpr("cp", permCols, "", "USERNAME", "USER_NAME") + `,` + strExpr("cp", permCols, "", "GROUPNAME", "GROUP_NAME") + ` FROM CONTENT_PERM cp JOIN CONTENT_PERM_SET cps ON cps.` + "`" + setIDColumn + "`" + `=cp.` + "`" + setFKColumn + "`" + ` ORDER BY cp.` + "`" + idColumn + "`"
	rows, err := r.source.db.QueryContext(ctx, query)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var legacyID, contentLegacy, rawPermission, username, groupName string
		if err := rows.Scan(&legacyID, &contentLegacy, &rawPermission, &username, &groupName); err != nil {
			return err
		}
		permission, supported := canonicalPagePermission(rawPermission)
		target := deterministicID("PAGE_PERMISSION", legacyID)
		details := map[string]any{"contentLegacyId": contentLegacy, "sourcePermission": rawPermission, "permission": permission, "username": username, "groupName": groupName}
		if err := r.apply(ctx, "PAGE_PERMISSION", legacyID, target, details, func() error {
			if !supported {
				return fmt.Errorf("unsupported Confluence page permission %q", rawPermission)
			}
			pageID, err := r.permissionPage(ctx, contentLegacy)
			if err != nil {
				return err
			}
			subjectType, subjectID, err := r.permissionSubject(ctx, username, groupName)
			if err != nil {
				return err
			}
			_, err = r.service.Store.Pool.Exec(ctx, `INSERT INTO page_permissions(id,page_id,permission,subject_type,subject_id,legacy_system,legacy_id) VALUES($1,$2,$3,$4,$5,'CONFLUENCE',$6) ON CONFLICT(id) DO UPDATE SET page_id=excluded.page_id,permission=excluded.permission,subject_type=excluded.subject_type,subject_id=excluded.subject_id,legacy_system='CONFLUENCE',legacy_id=excluded.legacy_id`, target, pageID, permission, subjectType, subjectID, legacyID)
			return err
		}); err != nil {
			return err
		}
	}
	return rows.Err()
}

func (r *snapshotRunner) permissionSpace(ctx context.Context, legacyID string) (uuid.UUID, error) {
	if id, ok := r.spacesByLegacy[legacyID]; ok {
		return id, nil
	}
	var id uuid.UUID
	err := r.service.Store.Pool.QueryRow(ctx, `SELECT target_id FROM migration_mapping WHERE legacy_system='CONFLUENCE' AND entity_type='SPACE' AND legacy_id=$1`, legacyID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, fmt.Errorf("space %s was not migrated", legacyID)
	}
	return id, err
}

func (r *snapshotRunner) permissionPage(ctx context.Context, legacyID string) (uuid.UUID, error) {
	if id, ok := r.pageTargets[legacyID]; ok {
		return id, nil
	}
	var id uuid.UUID
	err := r.service.Store.Pool.QueryRow(ctx, `SELECT target_id FROM migration_mapping WHERE legacy_system='CONFLUENCE' AND entity_type='PAGE' AND legacy_id=$1`, legacyID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, fmt.Errorf("page %s was not migrated", legacyID)
	}
	return id, err
}

func (r *snapshotRunner) permissionSubject(ctx context.Context, username, groupName string) (string, uuid.UUID, error) {
	username = strings.TrimSpace(username)
	groupName = strings.TrimSpace(groupName)
	if username != "" && groupName != "" {
		return "", uuid.Nil, errors.New("permission has both a user and a group subject")
	}
	if username != "" {
		if id, ok := r.usersByName[strings.ToLower(username)]; ok {
			return "USER", id, nil
		}
		var id uuid.UUID
		err := r.service.Store.Pool.QueryRow(ctx, `SELECT id FROM users WHERE lower(username)=lower($1)`, username).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			return "", uuid.Nil, fmt.Errorf("permission user %q was not migrated", username)
		}
		return "USER", id, err
	}
	if groupName != "" {
		if id, ok := r.groupsByName[strings.ToLower(groupName)]; ok {
			return "GROUP", id, nil
		}
		var id uuid.UUID
		err := r.service.Store.Pool.QueryRow(ctx, `SELECT id FROM groups WHERE lower(name)=lower($1)`, groupName).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			return "", uuid.Nil, fmt.Errorf("permission group %q was not migrated", groupName)
		}
		return "GROUP", id, err
	}
	return "", uuid.Nil, errors.New("permission has no resolvable user or group subject; anonymous access is not granted automatically")
}

func canonicalSpacePermission(value string) (string, bool) {
	permission := strings.ToUpper(strings.TrimSpace(value))
	mapping := map[string]string{
		"VIEWSPACE": "VIEW", "VIEW": "VIEW",
		"ADDPAGE": "CREATE", "CREATE": "CREATE",
		"EDITSPACE": "EDIT", "EDITPAGE": "EDIT", "EDIT": "EDIT",
		"REMOVEPAGE": "DELETE", "REMOVEOWNCONTENT": "DELETE", "DELETE": "DELETE",
		"COMMENT":          "COMMENT",
		"CREATEATTACHMENT": "ATTACH", "REMOVEATTACHMENT": "ATTACH", "ATTACH": "ATTACH",
		"EXPORTSPACE": "EXPORT", "EXPORT": "EXPORT",
		"SETPAGEPERMISSIONS": "RESTRICT", "RESTRICT": "RESTRICT",
		"ADMINISTERSPACE": "SPACE_ADMIN", "SPACE_ADMIN": "SPACE_ADMIN",
	}
	canonical, ok := mapping[permission]
	return canonical, ok
}

func canonicalPagePermission(value string) (string, bool) {
	permission := strings.ToUpper(strings.TrimSpace(value))
	switch permission {
	case "VIEW", "EDIT":
		return permission, true
	default:
		return "", false
	}
}

func (r *snapshotRunner) resolveSpace(ctx context.Context, legacyID string) (uuid.UUID, error) {
	if id, ok := r.spacesByLegacy[legacyID]; ok {
		return id, nil
	}
	if legacyID != "" {
		var target uuid.UUID
		if err := r.service.Store.Pool.QueryRow(ctx, `SELECT target_id FROM migration_mapping WHERE legacy_system='CONFLUENCE' AND entity_type='SPACE' AND legacy_id=$1`, legacyID).Scan(&target); err == nil {
			return target, nil
		}
	}
	target := deterministicID("SPACE", "__ORPHAN__")
	_, err := r.service.Store.Pool.Exec(ctx, `INSERT INTO spaces(id,space_key,name,description,status,legacy_system,legacy_id) VALUES($1,'MIGRATED','Migrated Content','Content whose legacy Space could not be resolved','ACTIVE','CONFLUENCE','__ORPHAN__') ON CONFLICT(id) DO NOTHING`, target)
	return target, err
}
func (r *snapshotRunner) userFor(key string) *uuid.UUID {
	if key == "" {
		return nil
	}
	if id, ok := r.usersByName[strings.ToLower(key)]; ok {
		return &id
	}
	if id, ok := r.usersByLegacy[key]; ok {
		return &id
	}
	return nil
}
func (r *snapshotRunner) unsupported(ctx context.Context, pageID uuid.UUID, legacyID, kind, name, sample string) {
	r.unsupportedCount(ctx, pageID, legacyID, kind, name, sample, 1)
}
func (r *snapshotRunner) unsupportedCount(ctx context.Context, pageID uuid.UUID, legacyID, kind, name, sample string, count int64) {
	_, _ = r.service.Store.Pool.Exec(ctx, `INSERT INTO unsupported_content(id,job_id,page_id,legacy_id,kind,name,sample,occurrence_count) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(job_id,page_id,kind,name) DO UPDATE SET occurrence_count=unsupported_content.occurrence_count+excluded.occurrence_count,sample=excluded.sample,updated_at=now()`, deterministicID("UNSUPPORTED", r.jobID.String()+":"+pageID.String()+":"+kind+":"+name), r.jobID, pageID, legacyID, kind, name, truncate(sample, 1000), count)
}

func (r *snapshotRunner) reconcile(ctx context.Context) error {
	_, err := r.service.Store.Pool.Exec(ctx, `DELETE FROM macro_compatibility`)
	if err != nil {
		return err
	}
	_, err = r.service.Store.Pool.Exec(ctx, `INSERT INTO macro_compatibility(macro_name,support_level,page_count,occurrence_count,conversion_rate) SELECT macro_name,CASE WHEN bool_and(supported) THEN 'NATIVE' ELSE 'UNSUPPORTED' END,count(DISTINCT page_id),sum(occurrence_count),CASE WHEN bool_and(supported) THEN 100 ELSE 0 END FROM page_macros GROUP BY macro_name`)
	if err != nil {
		return err
	}
	types := []struct {
		entity, category, name string
		enabled                bool
	}{{"USER", "USERS", "Users", r.options.IncludeUsers}, {"GROUP", "GROUPS", "Groups", r.options.IncludeGroups}, {"GROUP_MEMBER", "GROUPS", "Group Memberships", r.options.IncludeGroups}, {"SPACE", "SPACES", "Spaces", r.options.IncludeSpaces}, {"PAGE", "PAGES", "Pages", r.options.IncludePages}, {"COMMENT", "COMMENTS", "Comments", r.options.IncludeComments}, {"ATTACHMENT", "ATTACHMENTS", "Attachment Metadata", r.options.IncludeAttachmentMetadata}}
	for _, x := range types {
		if !x.enabled {
			continue
		}
		var sourceCount, completed, failed int64
		_ = r.service.Store.Pool.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE status='COMPLETE'),count(*) FILTER(WHERE status='FAILED') FROM migration_items WHERE job_id=$1 AND entity_type=$2`, r.jobID, x.entity).Scan(&sourceCount, &completed, &failed)
		targetCount, err := r.targetEntityCount(ctx, x.entity)
		if err != nil {
			return err
		}
		status := "PASS"
		if failed > 0 || completed != sourceCount || sourceCount != targetCount {
			status = "FAIL"
		}
		if err := r.upsertCheck(ctx, x.category, x.name, status, sourceCount, targetCount, failed, map[string]any{"jobId": r.jobID, "completedItems": completed}); err != nil {
			return err
		}
	}
	if r.options.IncludePages {
		var versionSource, versionCompleted, versionFailed int64
		_ = r.service.Store.Pool.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE status='COMPLETE'),count(*) FILTER(WHERE status='FAILED') FROM migration_items WHERE job_id=$1 AND entity_type IN ('PAGE','PAGE_VERSION')`, r.jobID).Scan(&versionSource, &versionCompleted, &versionFailed)
		var versionTarget int64
		if err := r.service.Store.Pool.QueryRow(ctx, `SELECT count(*) FROM page_versions WHERE legacy_system='CONFLUENCE'`).Scan(&versionTarget); err != nil {
			return err
		}
		versionStatus := "PASS"
		if versionSource != versionCompleted || versionSource != versionTarget || versionFailed > 0 {
			versionStatus = "FAIL"
		}
		if err := r.upsertCheck(ctx, "PAGE_VERSIONS", "Page Versions", versionStatus, versionSource, versionTarget, versionFailed, map[string]any{"jobId": r.jobID, "completedItems": versionCompleted}); err != nil {
			return err
		}
		var unsupported int64
		_ = r.service.Store.Pool.QueryRow(ctx, `SELECT count(*) FROM unsupported_content WHERE job_id=$1 AND status='OPEN'`, r.jobID).Scan(&unsupported)
		macroStatus := "PASS"
		if unsupported > 0 {
			macroStatus = "WARNING"
		}
		if err := r.upsertCheck(ctx, "MACROS", "Macro Compatibility", macroStatus, nil, nil, unsupported, map[string]any{"jobId": r.jobID}); err != nil {
			return err
		}
		var orphan int64
		_ = r.service.Store.Pool.QueryRow(ctx, `SELECT count(*) FROM unsupported_content WHERE job_id=$1 AND kind='ORPHAN_PAGE' AND status='OPEN'`, r.jobID).Scan(&orphan)
		hierarchyStatus := "PASS"
		if orphan > 0 {
			hierarchyStatus = "FAIL"
		}
		if err := r.upsertCheck(ctx, "PAGE_HIERARCHY", "Page Tree", hierarchyStatus, nil, nil, orphan, map[string]any{"jobId": r.jobID}); err != nil {
			return err
		}
		var hashSource, hashTarget, hashMismatch int64
		_ = r.service.Store.Pool.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE status='COMPLETE' AND target_hash<>''),count(*) FILTER(WHERE status<>'COMPLETE' OR source_hash='' OR target_hash='' OR source_hash<>target_hash) FROM migration_items WHERE job_id=$1 AND entity_type IN ('PAGE','PAGE_VERSION')`, r.jobID).Scan(&hashSource, &hashTarget, &hashMismatch)
		hashStatus := "PASS"
		if hashMismatch > 0 || hashSource != hashTarget {
			hashStatus = "FAIL"
		}
		if err := r.upsertCheck(ctx, "CONTENT_HASH", "Converted Content Hash", hashStatus, hashSource, hashTarget, hashMismatch, map[string]any{"algorithm": "SHA-256", "jobId": r.jobID}); err != nil {
			return err
		}
		_, _ = r.service.Store.Pool.Exec(ctx, `UPDATE page_links pl SET target_page_id=(SELECT p.id FROM pages p WHERE lower(p.title)=lower(pl.target) AND p.deleted_at IS NULL ORDER BY p.updated_at DESC LIMIT 1) WHERE pl.link_type='PAGE' AND pl.target_page_id IS NULL`)
		var brokenLinks int64
		_ = r.service.Store.Pool.QueryRow(ctx, `SELECT count(*) FROM page_links WHERE link_type='PAGE' AND target_page_id IS NULL`).Scan(&brokenLinks)
		linkStatus := "PASS"
		if brokenLinks > 0 {
			linkStatus = "WARNING"
		}
		if err := r.upsertCheck(ctx, "INTERNAL_LINKS", "Internal Links", linkStatus, nil, nil, brokenLinks, map[string]any{"jobId": r.jobID}); err != nil {
			return err
		}
	}
	if r.options.IncludeAttachmentMetadata {
		var attachmentCount int64
		_ = r.service.Store.Pool.QueryRow(ctx, `SELECT count(*) FROM attachments WHERE legacy_system='CONFLUENCE'`).Scan(&attachmentCount)
		binaryStatus := "PASS"
		if attachmentCount > 0 {
			binaryStatus = "WARNING"
		}
		if err := r.upsertCheck(ctx, "ATTACHMENT_BINARY", "Attachment Binary Hash", binaryStatus, attachmentCount, int64(0), attachmentCount, map[string]any{"reason": "binary copy has not run", "jobId": r.jobID}); err != nil {
			return err
		}
	}
	if r.options.IncludePermissions {
		var permissionSource, permissionCompleted, permissionFailed int64
		_ = r.service.Store.Pool.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE status='COMPLETE'),count(*) FILTER(WHERE status='FAILED') FROM migration_items WHERE job_id=$1 AND entity_type IN ('SPACE_PERMISSION','PAGE_PERMISSION')`, r.jobID).Scan(&permissionSource, &permissionCompleted, &permissionFailed)
		var permissionTarget int64
		if err := r.service.Store.Pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM space_permissions WHERE legacy_system='CONFLUENCE')+(SELECT count(*) FROM page_permissions WHERE legacy_system='CONFLUENCE')`).Scan(&permissionTarget); err != nil {
			return err
		}
		permissionStatus := "PASS"
		if permissionSource != permissionCompleted || permissionSource != permissionTarget || permissionFailed > 0 {
			permissionStatus = "FAIL"
		}
		if err := r.upsertCheck(ctx, "PERMISSIONS", "Permission Equivalence", permissionStatus, permissionSource, permissionTarget, permissionFailed, map[string]any{"jobId": r.jobID, "completedItems": permissionCompleted, "policy": "unresolved and anonymous subjects fail closed"}); err != nil {
			return err
		}
	}
	_, err = r.service.Store.Pool.Exec(ctx, `UPDATE migration_state SET readiness=round((SELECT count(*) FROM migration_checks WHERE status IN ('PASS','APPROVED'))::numeric/GREATEST((SELECT count(*) FROM migration_checks),13)*100,2),updated_at=now() WHERE id=true`)
	return err
}
func (r *snapshotRunner) targetEntityCount(ctx context.Context, entity string) (int64, error) {
	queries := map[string]string{
		"USER":         `SELECT count(*) FROM users WHERE legacy_system='CONFLUENCE'`,
		"GROUP":        `SELECT count(*) FROM groups WHERE legacy_system='CONFLUENCE'`,
		"GROUP_MEMBER": `SELECT count(*) FROM group_members gm JOIN groups g ON g.id=gm.group_id JOIN users u ON u.id=gm.user_id WHERE g.legacy_system='CONFLUENCE' AND u.legacy_system='CONFLUENCE'`,
		"SPACE":        `SELECT count(*) FROM spaces WHERE legacy_system='CONFLUENCE' AND legacy_id<>'__ORPHAN__'`,
		"PAGE":         `SELECT count(*) FROM pages WHERE legacy_system='CONFLUENCE'`,
		"COMMENT":      `SELECT count(*) FROM comments WHERE legacy_system='CONFLUENCE'`,
		"ATTACHMENT":   `SELECT count(*) FROM attachments WHERE legacy_system='CONFLUENCE'`,
	}
	query := queries[entity]
	if query == "" {
		return 0, fmt.Errorf("no target count query for %s", entity)
	}
	var count int64
	err := r.service.Store.Pool.QueryRow(ctx, query).Scan(&count)
	return count, err
}
func (r *snapshotRunner) upsertCheck(ctx context.Context, category, name, status string, source, target any, mismatch int64, details any) error {
	raw, _ := json.Marshal(details)
	_, err := r.service.Store.Pool.Exec(ctx, `INSERT INTO migration_checks(id,category,check_name,status,source_count,target_count,mismatch_count,details) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(category,check_name) DO UPDATE SET status=excluded.status,source_count=excluded.source_count,target_count=excluded.target_count,mismatch_count=excluded.mismatch_count,details=excluded.details,checked_at=now()`, uuid.New(), category, name, status, source, target, mismatch, raw)
	return err
}

func deterministicID(entity, legacyID string) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("kanvas:confluence:"+entity+":"+legacyID))
}
func strExpr(alias string, cols map[string]bool, fallback string, names ...string) string {
	for _, name := range names {
		if cols[strings.ToUpper(name)] {
			return fmt.Sprintf("COALESCE(CAST(%s.`%s` AS CHAR),'%s')", alias, name, sqlLiteral(fallback))
		}
	}
	return "'" + sqlLiteral(fallback) + "'"
}
func intExpr(alias string, cols map[string]bool, fallback int, names ...string) string {
	for _, name := range names {
		if cols[strings.ToUpper(name)] {
			return fmt.Sprintf("COALESCE(%s.`%s`,%d)", alias, name, fallback)
		}
	}
	return fmt.Sprintf("%d", fallback)
}
func firstColumn(cols map[string]bool, names ...string) string {
	for _, name := range names {
		name = strings.ToUpper(name)
		if cols[name] {
			return name
		}
	}
	return ""
}
func dateExpr(alias string, cols map[string]bool, name string) string {
	if cols[strings.ToUpper(name)] {
		return fmt.Sprintf("COALESCE(DATE_FORMAT(%s.`%s`,'%%Y-%%m-%%dT%%H:%%i:%%s.%%f'),'')", alias, name)
	}
	return `''`
}
func sqlLiteral(v string) string { return strings.ReplaceAll(v, "'", "''") }
func parseLegacyTime(v string) time.Time {
	layouts := []string{"2006-01-02T15:04:05.000000", "2006-01-02T15:04:05", "2006-01-02 15:04:05"}
	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, v, time.UTC); err == nil {
			return t
		}
	}
	return time.Now().UTC()
}
func withoutContent(in []*legacyContent, id string) []*legacyContent {
	out := make([]*legacyContent, 0, len(in))
	for _, v := range in {
		if v.ID != id {
			out = append(out, v)
		}
	}
	return out
}
func latestContent(in []*legacyContent) *legacyContent {
	latest := in[0]
	for _, v := range in {
		if strings.EqualFold(v.Status, "current") || v.Version > latest.Version {
			latest = v
		}
	}
	return latest
}
func truncate(v string, n int) string {
	if len(v) <= n {
		return v
	}
	return v[:n]
}
