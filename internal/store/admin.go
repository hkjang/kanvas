package store

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type AdminOverview struct {
	Users          int64      `json:"users"`
	ActiveUsers    int64      `json:"activeUsers"`
	Administrators int64      `json:"administrators"`
	Groups         int64      `json:"groups"`
	Spaces         int64      `json:"spaces"`
	ArchivedSpaces int64      `json:"archivedSpaces"`
	Pages          int64      `json:"pages"`
	Attachments    int64      `json:"attachments"`
	ActiveSessions int64      `json:"activeSessions"`
	ActiveAPIKeys  int64      `json:"activeApiKeys"`
	AuditEvents24H int64      `json:"auditEvents24h"`
	OpenExceptions int64      `json:"openExceptions"`
	LastActivityAt *time.Time `json:"lastActivityAt,omitempty"`
}

type AdminUser struct {
	User
	CreatedAt  time.Time `json:"createdAt"`
	GroupCount int64     `json:"groupCount"`
}

type AdminGroup struct {
	ID           uuid.UUID `json:"id"`
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	MemberCount  int64     `json:"memberCount"`
	LegacySystem string    `json:"legacySystem"`
	CreatedAt    time.Time `json:"createdAt"`
}

type AdminSpace struct {
	Space
	PageCount       int64     `json:"pageCount"`
	AttachmentCount int64     `json:"attachmentCount"`
	CreatedAt       time.Time `json:"createdAt"`
}

func (s *Store) AdminOverview(ctx context.Context) (AdminOverview, error) {
	var value AdminOverview
	err := s.Pool.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM users),
		(SELECT count(*) FROM users WHERE status='ACTIVE'),
		(SELECT count(*) FROM users WHERE role='ADMIN' AND status='ACTIVE'),
		(SELECT count(*) FROM groups),
		(SELECT count(*) FROM spaces WHERE status<>'DELETED'),
		(SELECT count(*) FROM spaces WHERE status='ARCHIVED'),
		(SELECT count(*) FROM pages WHERE deleted_at IS NULL),
		(SELECT count(*) FROM attachments),
		(SELECT count(*) FROM sessions WHERE expires_at>now()),
		(SELECT count(*) FROM api_keys WHERE revoked_at IS NULL AND (expires_at IS NULL OR expires_at>now())),
		(SELECT count(*) FROM audit_events WHERE created_at>now()-interval '24 hours'),
		(SELECT count(*) FROM unsupported_content WHERE status='OPEN' AND job_id=(SELECT id FROM migration_jobs WHERE kind='SNAPSHOT' ORDER BY created_at DESC LIMIT 1)),
		(SELECT max(created_at) FROM audit_events)`).Scan(
		&value.Users, &value.ActiveUsers, &value.Administrators, &value.Groups,
		&value.Spaces, &value.ArchivedSpaces, &value.Pages, &value.Attachments,
		&value.ActiveSessions, &value.ActiveAPIKeys, &value.AuditEvents24H,
		&value.OpenExceptions, &value.LastActivityAt,
	)
	return value, err
}

func (s *Store) AdminUsers(ctx context.Context, query string) ([]AdminUser, error) {
	query = strings.TrimSpace(query)
	rows, err := s.Pool.Query(ctx, `SELECT u.`+strings.ReplaceAll(userColumns, ",", ",u.")+`,u.created_at,count(gm.group_id)
		FROM users u LEFT JOIN group_members gm ON gm.user_id=u.id
		WHERE $1='' OR u.username ILIKE '%'||$1||'%' OR u.display_name ILIKE '%'||$1||'%' OR u.email ILIKE '%'||$1||'%'
		GROUP BY u.id ORDER BY CASE WHEN u.role='ADMIN' THEN 0 ELSE 1 END,u.display_name,u.username LIMIT 500`, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]AdminUser, 0)
	for rows.Next() {
		var value AdminUser
		if err := rows.Scan(&value.ID, &value.Username, &value.DisplayName, &value.Email, &value.Role, &value.Status, &value.IdentityProvider, &value.LastLoginAt, &value.CreatedAt, &value.GroupCount); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (s *Store) UpdateAdminUser(ctx context.Context, actorID, userID uuid.UUID, role, status string) (AdminUser, error) {
	role = strings.ToUpper(strings.TrimSpace(role))
	status = strings.ToUpper(strings.TrimSpace(status))
	if role != "USER" && role != "ADMIN" {
		return AdminUser{}, errors.New("role must be USER or ADMIN")
	}
	if status != "ACTIVE" && status != "DISABLED" {
		return AdminUser{}, errors.New("status must be ACTIVE or DISABLED")
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return AdminUser{}, err
	}
	defer tx.Rollback(ctx)
	// Serialize administrator policy changes so concurrent demotions cannot remove every active administrator.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('kanvas_admin_role_policy'))`); err != nil {
		return AdminUser{}, err
	}
	var currentRole, currentStatus string
	if err = tx.QueryRow(ctx, `SELECT role,status FROM users WHERE id=$1 FOR UPDATE`, userID).Scan(&currentRole, &currentStatus); err != nil {
		return AdminUser{}, err
	}
	if actorID == userID && status == "DISABLED" {
		return AdminUser{}, errors.New("you cannot disable your own administrator account")
	}
	if currentRole == "ADMIN" && currentStatus == "ACTIVE" && (role != "ADMIN" || status != "ACTIVE") {
		var administrators int
		if err = tx.QueryRow(ctx, `SELECT count(*) FROM users WHERE role='ADMIN' AND status='ACTIVE'`).Scan(&administrators); err != nil {
			return AdminUser{}, err
		}
		if administrators <= 1 {
			return AdminUser{}, errors.New("the last active administrator cannot be demoted or disabled")
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE users SET role=$2,status=$3,updated_at=now() WHERE id=$1`, userID, role, status); err != nil {
		return AdminUser{}, err
	}
	if status == "DISABLED" {
		if _, err = tx.Exec(ctx, `DELETE FROM sessions WHERE user_id=$1`, userID); err != nil {
			return AdminUser{}, err
		}
		if _, err = tx.Exec(ctx, `UPDATE api_keys SET revoked_at=coalesce(revoked_at,now()) WHERE user_id=$1`, userID); err != nil {
			return AdminUser{}, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return AdminUser{}, err
	}
	return s.AdminUserByID(ctx, userID)
}

func (s *Store) AdminUserByID(ctx context.Context, userID uuid.UUID) (AdminUser, error) {
	var value AdminUser
	err := s.Pool.QueryRow(ctx, `SELECT u.`+strings.ReplaceAll(userColumns, ",", ",u.")+`,u.created_at,(SELECT count(*) FROM group_members gm WHERE gm.user_id=u.id) FROM users u WHERE u.id=$1`, userID).Scan(
		&value.ID, &value.Username, &value.DisplayName, &value.Email, &value.Role, &value.Status, &value.IdentityProvider, &value.LastLoginAt, &value.CreatedAt, &value.GroupCount,
	)
	return value, err
}

func (s *Store) AdminGroups(ctx context.Context) ([]AdminGroup, error) {
	rows, err := s.Pool.Query(ctx, `SELECT g.id,g.name,g.description,count(gm.user_id),coalesce(g.legacy_system,''),g.created_at FROM groups g LEFT JOIN group_members gm ON gm.group_id=g.id GROUP BY g.id ORDER BY g.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]AdminGroup, 0)
	for rows.Next() {
		var value AdminGroup
		if err := rows.Scan(&value.ID, &value.Name, &value.Description, &value.MemberCount, &value.LegacySystem, &value.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (s *Store) CreateAdminGroup(ctx context.Context, name, description string) (AdminGroup, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return AdminGroup{}, errors.New("group name is required")
	}
	id := uuid.New()
	_, err := s.Pool.Exec(ctx, `INSERT INTO groups(id,name,description) VALUES($1,$2,$3)`, id, name, strings.TrimSpace(description))
	if err != nil {
		return AdminGroup{}, err
	}
	return s.AdminGroupByID(ctx, id)
}

func (s *Store) AdminGroupByID(ctx context.Context, groupID uuid.UUID) (AdminGroup, error) {
	var value AdminGroup
	err := s.Pool.QueryRow(ctx, `SELECT g.id,g.name,g.description,(SELECT count(*) FROM group_members gm WHERE gm.group_id=g.id),coalesce(g.legacy_system,''),g.created_at FROM groups g WHERE g.id=$1`, groupID).Scan(&value.ID, &value.Name, &value.Description, &value.MemberCount, &value.LegacySystem, &value.CreatedAt)
	return value, err
}

func (s *Store) AdminGroupMembers(ctx context.Context, groupID uuid.UUID) ([]AdminUser, error) {
	rows, err := s.Pool.Query(ctx, `SELECT u.`+strings.ReplaceAll(userColumns, ",", ",u.")+`,u.created_at,(SELECT count(*) FROM group_members x WHERE x.user_id=u.id) FROM users u JOIN group_members gm ON gm.user_id=u.id WHERE gm.group_id=$1 ORDER BY u.display_name,u.username`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]AdminUser, 0)
	for rows.Next() {
		var value AdminUser
		if err := rows.Scan(&value.ID, &value.Username, &value.DisplayName, &value.Email, &value.Role, &value.Status, &value.IdentityProvider, &value.LastLoginAt, &value.CreatedAt, &value.GroupCount); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (s *Store) AddAdminGroupMember(ctx context.Context, groupID, userID uuid.UUID) error {
	_, err := s.Pool.Exec(ctx, `INSERT INTO group_members(group_id,user_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, groupID, userID)
	return err
}

func (s *Store) RemoveAdminGroupMember(ctx context.Context, groupID, userID uuid.UUID) error {
	command, err := s.Pool.Exec(ctx, `DELETE FROM group_members WHERE group_id=$1 AND user_id=$2`, groupID, userID)
	if err == nil && command.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return err
}

func (s *Store) AdminSpaces(ctx context.Context) ([]AdminSpace, error) {
	rows, err := s.Pool.Query(ctx, `SELECT s.id,s.space_key,s.name,s.description,s.status,s.updated_at,
		(SELECT count(*) FROM pages p WHERE p.space_id=s.id AND p.deleted_at IS NULL),
		(SELECT count(*) FROM attachments a JOIN pages p ON p.id=a.page_id WHERE p.space_id=s.id),s.created_at
		FROM spaces s WHERE s.status<>'DELETED' ORDER BY CASE WHEN s.status='ACTIVE' THEN 0 ELSE 1 END,s.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]AdminSpace, 0)
	for rows.Next() {
		var value AdminSpace
		if err := rows.Scan(&value.ID, &value.Key, &value.Name, &value.Description, &value.Status, &value.UpdatedAt, &value.PageCount, &value.AttachmentCount, &value.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (s *Store) UpdateAdminSpaceStatus(ctx context.Context, spaceID uuid.UUID, status string) (AdminSpace, error) {
	status = strings.ToUpper(strings.TrimSpace(status))
	if status != "ACTIVE" && status != "ARCHIVED" {
		return AdminSpace{}, errors.New("space status must be ACTIVE or ARCHIVED")
	}
	command, err := s.Pool.Exec(ctx, `UPDATE spaces SET status=$2,updated_at=now() WHERE id=$1 AND status<>'DELETED'`, spaceID, status)
	if err != nil {
		return AdminSpace{}, err
	}
	if command.RowsAffected() == 0 {
		return AdminSpace{}, pgx.ErrNoRows
	}
	var value AdminSpace
	err = s.Pool.QueryRow(ctx, `SELECT s.id,s.space_key,s.name,s.description,s.status,s.updated_at,
		(SELECT count(*) FROM pages p WHERE p.space_id=s.id AND p.deleted_at IS NULL),
		(SELECT count(*) FROM attachments a JOIN pages p ON p.id=a.page_id WHERE p.space_id=s.id),s.created_at FROM spaces s WHERE s.id=$1`, spaceID).Scan(
		&value.ID, &value.Key, &value.Name, &value.Description, &value.Status, &value.UpdatedAt, &value.PageCount, &value.AttachmentCount, &value.CreatedAt,
	)
	return value, err
}
