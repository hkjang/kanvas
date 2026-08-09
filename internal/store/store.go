package store

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

//go:embed schema.sql
var schemaSQL string

type Store struct {
	Pool *pgxpool.Pool
}

type User struct {
	ID               uuid.UUID  `json:"id"`
	Username         string     `json:"username"`
	DisplayName      string     `json:"displayName"`
	Email            string     `json:"email"`
	Role             string     `json:"role"`
	Status           string     `json:"status"`
	IdentityProvider string     `json:"identityProvider"`
	LastLoginAt      *time.Time `json:"lastLoginAt,omitempty"`
}

type Setting struct {
	Key         string          `json:"key"`
	Value       json.RawMessage `json:"value"`
	Secret      bool            `json:"secret"`
	Description string          `json:"description"`
	UpdatedAt   time.Time       `json:"updatedAt"`
}

type Space struct {
	ID          uuid.UUID `json:"id"`
	Key         string    `json:"key"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Status      string    `json:"status"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type Page struct {
	ID             uuid.UUID       `json:"id"`
	SpaceID        uuid.UUID       `json:"spaceId"`
	ParentID       *uuid.UUID      `json:"parentId,omitempty"`
	Title          string          `json:"title"`
	Status         string          `json:"status"`
	CurrentVersion int             `json:"currentVersion"`
	EditorDocument json.RawMessage `json:"editorDocument"`
	RenderedText   string          `json:"renderedText"`
	UpdatedAt      time.Time       `json:"updatedAt"`
	UpdatedBy      string          `json:"updatedBy"`
}

type APIKey struct {
	ID         uuid.UUID  `json:"id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	Scopes     []string   `json:"scopes"`
	ExpiresAt  *time.Time `json:"expiresAt,omitempty"`
	LastUsedAt *time.Time `json:"lastUsedAt,omitempty"`
	CreatedAt  time.Time  `json:"createdAt"`
	RevokedAt  *time.Time `json:"revokedAt,omitempty"`
}

type Preferences struct {
	Locale     string          `json:"locale"`
	Theme      string          `json:"theme"`
	EditorMode string          `json:"editorMode"`
	Settings   json.RawMessage `json:"settings"`
}

type PageVersion struct {
	ID             uuid.UUID       `json:"id"`
	Version        int             `json:"version"`
	Title          string          `json:"title"`
	EditorDocument json.RawMessage `json:"editorDocument"`
	RenderedText   string          `json:"renderedText"`
	ChangeMessage  string          `json:"changeMessage"`
	CreatedBy      string          `json:"createdBy"`
	CreatedAt      time.Time       `json:"createdAt"`
}

type Comment struct {
	ID         uuid.UUID  `json:"id"`
	PageID     uuid.UUID  `json:"pageId"`
	ParentID   *uuid.UUID `json:"parentId,omitempty"`
	Body       string     `json:"body"`
	ResolvedAt *time.Time `json:"resolvedAt,omitempty"`
	CreatedBy  string     `json:"createdBy"`
	CreatedAt  time.Time  `json:"createdAt"`
}

func Open(ctx context.Context, dsn string) (*Store, error) {
	poolConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse postgres dsn: %w", err)
	}
	poolConfig.MaxConns = 20
	poolConfig.MinConns = 2
	poolConfig.MaxConnLifetime = time.Hour
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	if err = pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return &Store{Pool: pool}, nil
}

func (s *Store) Close() { s.Pool.Close() }

func (s *Store) Migrate(ctx context.Context) error {
	if _, err := s.Pool.Exec(ctx, schemaSQL); err != nil {
		return fmt.Errorf("apply schema: %w", err)
	}
	_, err := s.Pool.Exec(ctx, `INSERT INTO schema_migrations(version) VALUES(1) ON CONFLICT DO NOTHING`)
	return err
}

func (s *Store) BootstrapAdmin(ctx context.Context, username, password string) error {
	var count int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM users WHERE role='ADMIN'`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	id := uuid.New()
	_, err = tx.Exec(ctx, `INSERT INTO users(id,username,display_name,role) VALUES($1,$2,$3,'ADMIN')`, id, username, username)
	if err != nil {
		return fmt.Errorf("create bootstrap admin: %w", err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO local_credentials(user_id,password_hash) VALUES($1,$2)`, id, string(hash)); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO audit_events(id,actor_id,action,resource_type,resource_id,detail) VALUES($1,$2,'BOOTSTRAP_ADMIN_CREATED','USER',$3,'{}')`, uuid.New(), id, id.String())
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) EnsureWelcome(ctx context.Context, username string) error {
	var count int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM spaces`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	var userID uuid.UUID
	if err := s.Pool.QueryRow(ctx, `SELECT id FROM users WHERE username=$1`, username).Scan(&userID); err != nil {
		return err
	}
	space, err := s.CreateSpace(ctx, userID, "KANVAS", "Kanvas 시작하기", "Kanvas 서비스의 시작 공간입니다.")
	if err != nil {
		return err
	}
	doc := json.RawMessage(`{"type":"doc","content":[{"type":"heading","attrs":{"level":1},"content":[{"type":"text","text":"Kanvas에 오신 것을 환영합니다"}]},{"type":"paragraph","content":[{"type":"text","text":"관리자는 서비스 관리에서 Keycloak OIDC와 Confluence 마이그레이션을 설정할 수 있습니다."}]}]}`)
	_, err = s.CreatePage(ctx, userID, space.ID, nil, "Kanvas에 오신 것을 환영합니다", doc, "Kanvas에 오신 것을 환영합니다\n관리자는 서비스 관리에서 Keycloak OIDC와 Confluence 마이그레이션을 설정할 수 있습니다.")
	return err
}

func scanUser(row pgx.Row) (User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Username, &u.DisplayName, &u.Email, &u.Role, &u.Status, &u.IdentityProvider, &u.LastLoginAt)
	return u, err
}

const userColumns = `id,username,display_name,email,role,status,identity_provider,last_login_at`

func (s *Store) AuthenticateLocal(ctx context.Context, username, password string) (User, error) {
	u, err := scanUser(s.Pool.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE lower(username)=lower($1) AND status='ACTIVE'`, username))
	if err != nil {
		return User{}, err
	}
	var hash string
	if err = s.Pool.QueryRow(ctx, `SELECT password_hash FROM local_credentials WHERE user_id=$1`, u.ID).Scan(&hash); err != nil {
		return User{}, err
	}
	if err = bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return User{}, errors.New("invalid credentials")
	}
	_, _ = s.Pool.Exec(ctx, `UPDATE users SET last_login_at=now() WHERE id=$1`, u.ID)
	return u, nil
}

func (s *Store) UserByID(ctx context.Context, id uuid.UUID) (User, error) {
	return scanUser(s.Pool.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE id=$1`, id))
}

func (s *Store) UpsertOIDCUser(ctx context.Context, subject, username, displayName, email, adminGroup string, groups []string) (User, error) {
	role := "USER"
	for _, group := range groups {
		if adminGroup != "" && group == adminGroup {
			role = "ADMIN"
		}
	}
	if displayName == "" {
		displayName = username
	}
	id := uuid.New()
	_, err := s.Pool.Exec(ctx, `INSERT INTO users(id,username,display_name,email,role,identity_provider,external_subject,last_login_at)
		VALUES($1,$2,$3,$4,$5,'OIDC',$6,now())
		ON CONFLICT(identity_provider,external_subject) DO UPDATE SET username=excluded.username,display_name=excluded.display_name,email=excluded.email,role=excluded.role,status='ACTIVE',last_login_at=now(),updated_at=now()`,
		id, username, displayName, email, role, subject)
	if err != nil {
		return User{}, err
	}
	return scanUser(s.Pool.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE identity_provider='OIDC' AND external_subject=$1`, subject))
}

func (s *Store) OIDCUser(ctx context.Context, subject string) (User, error) {
	return scanUser(s.Pool.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE identity_provider='OIDC' AND external_subject=$1`, subject))
}

func (s *Store) CreateSession(ctx context.Context, userID uuid.UUID, tokenHash, csrf, remote, agent string, expiry time.Time) error {
	_, err := s.Pool.Exec(ctx, `INSERT INTO sessions(id,user_id,token_hash,csrf_token,expires_at,remote_addr,user_agent) VALUES($1,$2,$3,$4,$5,$6,$7)`, uuid.New(), userID, tokenHash, csrf, expiry, remote, agent)
	return err
}

func (s *Store) SessionUser(ctx context.Context, tokenHash string) (User, string, error) {
	row := s.Pool.QueryRow(ctx, `SELECT u.`+strings.ReplaceAll(userColumns, ",", ",u.")+`,s.csrf_token FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.token_hash=$1 AND s.expires_at>now() AND u.status='ACTIVE'`, tokenHash)
	var u User
	var csrf string
	err := row.Scan(&u.ID, &u.Username, &u.DisplayName, &u.Email, &u.Role, &u.Status, &u.IdentityProvider, &u.LastLoginAt, &csrf)
	if err == nil {
		_, _ = s.Pool.Exec(ctx, `UPDATE sessions SET last_seen_at=now() WHERE token_hash=$1`, tokenHash)
	}
	return u, csrf, err
}

func (s *Store) DeleteSession(ctx context.Context, tokenHash string) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM sessions WHERE token_hash=$1`, tokenHash)
	return err
}

func (s *Store) Settings(ctx context.Context) ([]Setting, error) {
	rows, err := s.Pool.Query(ctx, `SELECT key,value,secret,description,updated_at FROM system_settings ORDER BY key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Setting
	for rows.Next() {
		var v Setting
		if err := rows.Scan(&v.Key, &v.Value, &v.Secret, &v.Description, &v.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, rows.Err()
}

func (s *Store) Setting(ctx context.Context, key string) (json.RawMessage, bool, error) {
	var value json.RawMessage
	var secret bool
	err := s.Pool.QueryRow(ctx, `SELECT value,secret FROM system_settings WHERE key=$1`, key).Scan(&value, &secret)
	return value, secret, err
}

func (s *Store) PutSetting(ctx context.Context, key string, value any, secret bool, description string, actor uuid.UUID) error {
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = s.Pool.Exec(ctx, `INSERT INTO system_settings(key,value,secret,description,updated_by) VALUES($1,$2,$3,$4,$5)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value,secret=excluded.secret,description=excluded.description,updated_by=excluded.updated_by,updated_at=now()`, key, b, secret, description, actor)
	return err
}

func (s *Store) Audit(ctx context.Context, actor *uuid.UUID, action, resourceType, resourceID, remote, agent string, detail any) {
	b, _ := json.Marshal(detail)
	_, _ = s.Pool.Exec(ctx, `INSERT INTO audit_events(id,actor_id,action,resource_type,resource_id,remote_addr,user_agent,detail) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, uuid.New(), actor, action, resourceType, resourceID, remote, agent, b)
}

func (s *Store) Health(ctx context.Context) error { return s.Pool.Ping(ctx) }

func (s *Store) CreateAPIKey(ctx context.Context, userID uuid.UUID, name, prefix, tokenHash string, scopes []string, expiresAt *time.Time, rotatedFrom *uuid.UUID) (APIKey, error) {
	id := uuid.New()
	_, err := s.Pool.Exec(ctx, `INSERT INTO api_keys(id,user_id,name,key_prefix,token_hash,scopes,expires_at,rotated_from) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, id, userID, name, prefix, tokenHash, scopes, expiresAt, rotatedFrom)
	if err != nil {
		return APIKey{}, err
	}
	return s.APIKeyByID(ctx, userID, id)
}

func (s *Store) APIKeyByID(ctx context.Context, userID, id uuid.UUID) (APIKey, error) {
	var k APIKey
	err := s.Pool.QueryRow(ctx, `SELECT id,name,key_prefix,scopes,expires_at,last_used_at,created_at,revoked_at FROM api_keys WHERE id=$1 AND user_id=$2`, id, userID).Scan(&k.ID, &k.Name, &k.Prefix, &k.Scopes, &k.ExpiresAt, &k.LastUsedAt, &k.CreatedAt, &k.RevokedAt)
	return k, err
}

func (s *Store) APIKeys(ctx context.Context, userID uuid.UUID) ([]APIKey, error) {
	rows, err := s.Pool.Query(ctx, `SELECT id,name,key_prefix,scopes,expires_at,last_used_at,created_at,revoked_at FROM api_keys WHERE user_id=$1 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []APIKey
	for rows.Next() {
		var k APIKey
		if err := rows.Scan(&k.ID, &k.Name, &k.Prefix, &k.Scopes, &k.ExpiresAt, &k.LastUsedAt, &k.CreatedAt, &k.RevokedAt); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

func (s *Store) APIKeyUser(ctx context.Context, tokenHash string) (User, []string, error) {
	row := s.Pool.QueryRow(ctx, `SELECT u.`+strings.ReplaceAll(userColumns, ",", ",u.")+`,k.scopes,k.id FROM api_keys k JOIN users u ON u.id=k.user_id WHERE k.token_hash=$1 AND k.revoked_at IS NULL AND (k.expires_at IS NULL OR k.expires_at>now()) AND u.status='ACTIVE'`, tokenHash)
	var u User
	var scopes []string
	var keyID uuid.UUID
	err := row.Scan(&u.ID, &u.Username, &u.DisplayName, &u.Email, &u.Role, &u.Status, &u.IdentityProvider, &u.LastLoginAt, &scopes, &keyID)
	if err == nil {
		_, _ = s.Pool.Exec(ctx, `UPDATE api_keys SET last_used_at=now() WHERE id=$1`, keyID)
	}
	return u, scopes, err
}

func (s *Store) RevokeAPIKey(ctx context.Context, userID, keyID uuid.UUID) error {
	cmd, err := s.Pool.Exec(ctx, `UPDATE api_keys SET revoked_at=now() WHERE id=$1 AND user_id=$2 AND revoked_at IS NULL`, keyID, userID)
	if err == nil && cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return err
}

func (s *Store) RotateAPIKey(ctx context.Context, userID, keyID uuid.UUID, name, prefix, tokenHash string, scopes []string, expiresAt *time.Time) (APIKey, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return APIKey{}, err
	}
	defer tx.Rollback(ctx)
	cmd, err := tx.Exec(ctx, `UPDATE api_keys SET revoked_at=now() WHERE id=$1 AND user_id=$2 AND revoked_at IS NULL`, keyID, userID)
	if err != nil {
		return APIKey{}, err
	}
	if cmd.RowsAffected() == 0 {
		return APIKey{}, pgx.ErrNoRows
	}
	newID := uuid.New()
	_, err = tx.Exec(ctx, `INSERT INTO api_keys(id,user_id,name,key_prefix,token_hash,scopes,expires_at,rotated_from) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, newID, userID, name, prefix, tokenHash, scopes, expiresAt, keyID)
	if err != nil {
		return APIKey{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return APIKey{}, err
	}
	return s.APIKeyByID(ctx, userID, newID)
}

func (s *Store) Preferences(ctx context.Context, userID uuid.UUID) (Preferences, error) {
	var p Preferences
	err := s.Pool.QueryRow(ctx, `SELECT locale,theme,editor_mode,settings FROM user_preferences WHERE user_id=$1`, userID).Scan(&p.Locale, &p.Theme, &p.EditorMode, &p.Settings)
	if errors.Is(err, pgx.ErrNoRows) {
		return Preferences{Locale: "ko-KR", Theme: "system", EditorMode: "rich", Settings: json.RawMessage(`{}`)}, nil
	}
	return p, err
}

func (s *Store) PutPreferences(ctx context.Context, userID uuid.UUID, p Preferences) error {
	if len(p.Settings) == 0 {
		p.Settings = json.RawMessage(`{}`)
	}
	_, err := s.Pool.Exec(ctx, `INSERT INTO user_preferences(user_id,locale,theme,editor_mode,settings) VALUES($1,$2,$3,$4,$5) ON CONFLICT(user_id) DO UPDATE SET locale=excluded.locale,theme=excluded.theme,editor_mode=excluded.editor_mode,settings=excluded.settings,updated_at=now()`, userID, p.Locale, p.Theme, p.EditorMode, p.Settings)
	return err
}

func (s *Store) ListSpaces(ctx context.Context, userID uuid.UUID) ([]Space, error) {
	rows, err := s.Pool.Query(ctx, `SELECT s.id,s.space_key,s.name,s.description,s.status,s.updated_at FROM spaces s
		WHERE s.status<>'DELETED' AND (
		  EXISTS(SELECT 1 FROM users u WHERE u.id=$1 AND u.role='ADMIN') OR
		  NOT EXISTS(SELECT 1 FROM space_permissions sp WHERE sp.space_id=s.id AND sp.permission='VIEW') OR
		  EXISTS(SELECT 1 FROM space_permissions sp WHERE sp.space_id=s.id AND sp.permission='VIEW' AND ((sp.subject_type='USER' AND sp.subject_id=$1) OR (sp.subject_type='GROUP' AND EXISTS(SELECT 1 FROM group_members gm WHERE gm.group_id=sp.subject_id AND gm.user_id=$1))))
		) ORDER BY s.name`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Space
	for rows.Next() {
		var v Space
		if err := rows.Scan(&v.ID, &v.Key, &v.Name, &v.Description, &v.Status, &v.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) CreateSpace(ctx context.Context, userID uuid.UUID, key, name, description string) (Space, error) {
	v := Space{ID: uuid.New(), Key: strings.ToUpper(strings.TrimSpace(key)), Name: strings.TrimSpace(name), Description: description, Status: "ACTIVE"}
	err := s.Pool.QueryRow(ctx, `INSERT INTO spaces(id,space_key,name,description,created_by) VALUES($1,$2,$3,$4,$5) RETURNING updated_at`, v.ID, v.Key, v.Name, v.Description, userID).Scan(&v.UpdatedAt)
	return v, err
}

func scanPage(row pgx.Row) (Page, error) {
	var p Page
	err := row.Scan(&p.ID, &p.SpaceID, &p.ParentID, &p.Title, &p.Status, &p.CurrentVersion, &p.EditorDocument, &p.RenderedText, &p.UpdatedAt, &p.UpdatedBy)
	return p, err
}

const pageSelect = `SELECT p.id,p.space_id,p.parent_id,p.title,p.status,p.current_version,v.editor_document,v.rendered_text,p.updated_at,coalesce(u.display_name,'System') FROM pages p JOIN page_versions v ON v.page_id=p.id AND v.version=p.current_version LEFT JOIN users u ON u.id=p.updated_by`

func (s *Store) PagesInSpace(ctx context.Context, userID, spaceID uuid.UUID) ([]Page, error) {
	rows, err := s.Pool.Query(ctx, pageSelect+` WHERE p.space_id=$1 AND p.deleted_at IS NULL ORDER BY p.title`, spaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Page
	for rows.Next() {
		p, err := scanPage(rows)
		if err != nil {
			return nil, err
		}
		allowed, err := s.CanPage(ctx, userID, p.ID, "VIEW")
		if err != nil {
			return nil, err
		}
		if allowed {
			out = append(out, p)
		}
	}
	return out, rows.Err()
}

func (s *Store) PageByID(ctx context.Context, userID, pageID uuid.UUID) (Page, error) {
	allowed, err := s.CanPage(ctx, userID, pageID, "VIEW")
	if err != nil {
		return Page{}, err
	}
	if !allowed {
		return Page{}, pgx.ErrNoRows
	}
	return scanPage(s.Pool.QueryRow(ctx, pageSelect+` WHERE p.id=$1 AND p.deleted_at IS NULL`, pageID))
}

func (s *Store) CanPage(ctx context.Context, userID, pageID uuid.UUID, permission string) (bool, error) {
	var allowed bool
	err := s.Pool.QueryRow(ctx, `SELECT
		EXISTS(SELECT 1 FROM users WHERE id=$1 AND role='ADMIN') OR (
		  (NOT EXISTS(SELECT 1 FROM page_permissions pp WHERE pp.page_id=$2 AND pp.permission=$3) OR EXISTS(SELECT 1 FROM page_permissions pp WHERE pp.page_id=$2 AND pp.permission=$3 AND ((pp.subject_type='USER' AND pp.subject_id=$1) OR (pp.subject_type='GROUP' AND EXISTS(SELECT 1 FROM group_members gm WHERE gm.group_id=pp.subject_id AND gm.user_id=$1))))) AND
		  (NOT EXISTS(SELECT 1 FROM space_permissions sp JOIN pages p ON p.space_id=sp.space_id WHERE p.id=$2 AND sp.permission=$3) OR EXISTS(SELECT 1 FROM space_permissions sp JOIN pages p ON p.space_id=sp.space_id WHERE p.id=$2 AND sp.permission=$3 AND ((sp.subject_type='USER' AND sp.subject_id=$1) OR (sp.subject_type='GROUP' AND EXISTS(SELECT 1 FROM group_members gm WHERE gm.group_id=sp.subject_id AND gm.user_id=$1)))))
		)`, userID, pageID, permission).Scan(&allowed)
	return allowed, err
}

func (s *Store) CreatePage(ctx context.Context, userID, spaceID uuid.UUID, parentID *uuid.UUID, title string, doc json.RawMessage, text string) (Page, error) {
	allowed, err := s.CanSpace(ctx, userID, spaceID, "CREATE")
	if err != nil {
		return Page{}, err
	}
	if !allowed {
		return Page{}, errors.New("permission denied")
	}
	if len(doc) == 0 {
		doc = json.RawMessage(`{"type":"doc","content":[]}`)
	}
	pageID, versionID := uuid.New(), uuid.New()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Page{}, err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `INSERT INTO pages(id,space_id,parent_id,title,created_by,updated_by) VALUES($1,$2,$3,$4,$5,$5)`, pageID, spaceID, parentID, title, userID)
	if err != nil {
		return Page{}, err
	}
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(text)))
	_, err = tx.Exec(ctx, `INSERT INTO page_versions(id,page_id,version,title,editor_document,rendered_text,created_by,content_hash) VALUES($1,$2,1,$3,$4,$5,$6,$7)`, versionID, pageID, title, doc, text, userID, hash)
	if err != nil {
		return Page{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO wiki_change_journal(entity,entity_id,operation,new_value,user_id) VALUES('PAGE',$1,'CREATE',jsonb_build_object('title',$2::text),$3)`, pageID, title, userID)
	if err != nil {
		return Page{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Page{}, err
	}
	return s.PageByID(ctx, userID, pageID)
}

func (s *Store) CanSpace(ctx context.Context, userID, spaceID uuid.UUID, permission string) (bool, error) {
	var allowed bool
	err := s.Pool.QueryRow(ctx, `SELECT
		EXISTS(SELECT 1 FROM users WHERE id=$1 AND role='ADMIN') OR (
		  EXISTS(SELECT 1 FROM spaces WHERE id=$2 AND status<>'DELETED') AND
		  (NOT EXISTS(SELECT 1 FROM space_permissions WHERE space_id=$2 AND permission=$3) OR EXISTS(SELECT 1 FROM space_permissions sp WHERE sp.space_id=$2 AND sp.permission=$3 AND ((sp.subject_type='USER' AND sp.subject_id=$1) OR (sp.subject_type='GROUP' AND EXISTS(SELECT 1 FROM group_members gm WHERE gm.group_id=sp.subject_id AND gm.user_id=$1)))))
		)`, userID, spaceID, permission).Scan(&allowed)
	return allowed, err
}

func (s *Store) UpdatePage(ctx context.Context, userID, pageID uuid.UUID, title string, doc json.RawMessage, text, changeMessage string, expectedVersion int) (Page, error) {
	allowed, err := s.CanPage(ctx, userID, pageID, "EDIT")
	if err != nil {
		return Page{}, err
	}
	if !allowed {
		return Page{}, errors.New("permission denied")
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Page{}, err
	}
	defer tx.Rollback(ctx)
	var current int
	var oldTitle string
	err = tx.QueryRow(ctx, `SELECT current_version,title FROM pages WHERE id=$1 AND deleted_at IS NULL FOR UPDATE`, pageID).Scan(&current, &oldTitle)
	if err != nil {
		return Page{}, err
	}
	if current != expectedVersion {
		return Page{}, fmt.Errorf("version conflict: current version is %d", current)
	}
	next := current + 1
	cmd, err := tx.Exec(ctx, `UPDATE pages SET title=$2,current_version=$3,updated_by=$4,updated_at=now() WHERE id=$1`, pageID, title, next, userID)
	if err != nil || cmd.RowsAffected() == 0 {
		return Page{}, err
	}
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(text)))
	_, err = tx.Exec(ctx, `INSERT INTO page_versions(id,page_id,version,title,editor_document,rendered_text,change_message,created_by,content_hash) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, uuid.New(), pageID, next, title, doc, text, changeMessage, userID, hash)
	if err != nil {
		return Page{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO wiki_change_journal(entity,entity_id,operation,old_value,new_value,user_id) VALUES('PAGE',$1,'UPDATE',jsonb_build_object('title',$2::text,'version',$3::integer),jsonb_build_object('title',$4::text,'version',$5::integer),$6)`, pageID, oldTitle, current, title, next, userID)
	if err != nil {
		return Page{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Page{}, err
	}
	return s.PageByID(ctx, userID, pageID)
}

func (s *Store) PageVersions(ctx context.Context, userID, pageID uuid.UUID) ([]PageVersion, error) {
	rows, err := s.Pool.Query(ctx, `SELECT v.id,v.version,v.title,v.editor_document,v.rendered_text,v.change_message,coalesce(u.display_name,'System'),v.created_at FROM page_versions v LEFT JOIN users u ON u.id=v.created_by WHERE v.page_id=$1 ORDER BY v.version DESC`, pageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PageVersion
	for rows.Next() {
		var v PageVersion
		if err := rows.Scan(&v.ID, &v.Version, &v.Title, &v.EditorDocument, &v.RenderedText, &v.ChangeMessage, &v.CreatedBy, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) SearchPages(ctx context.Context, userID uuid.UUID, q string, limit int) ([]Page, error) {
	if limit < 1 || limit > 100 {
		limit = 30
	}
	pattern := "%" + q + "%"
	rows, err := s.Pool.Query(ctx, pageSelect+` WHERE p.deleted_at IS NULL AND (p.title ILIKE $1 OR v.rendered_text ILIKE $1 OR to_tsvector('simple',p.title||' '||v.rendered_text) @@ plainto_tsquery('simple',$2)) ORDER BY CASE WHEN p.title ILIKE $1 THEN 0 ELSE 1 END,p.updated_at DESC LIMIT $3`, pattern, q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Page
	for rows.Next() {
		p, err := scanPage(rows)
		if err != nil {
			return nil, err
		}
		allowed, err := s.CanPage(ctx, userID, p.ID, "VIEW")
		if err != nil {
			return nil, err
		}
		if allowed {
			out = append(out, p)
		}
	}
	return out, rows.Err()
}

func (s *Store) Comments(ctx context.Context, userID, pageID uuid.UUID) ([]Comment, error) {
	rows, err := s.Pool.Query(ctx, `SELECT c.id,c.page_id,c.parent_id,c.body,c.resolved_at,coalesce(u.display_name,'System'),c.created_at FROM comments c LEFT JOIN users u ON u.id=c.created_by WHERE c.page_id=$1 ORDER BY c.created_at`, pageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Comment
	for rows.Next() {
		var c Comment
		if err := rows.Scan(&c.ID, &c.PageID, &c.ParentID, &c.Body, &c.ResolvedAt, &c.CreatedBy, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) AddComment(ctx context.Context, userID, pageID uuid.UUID, parentID *uuid.UUID, body string) (Comment, error) {
	c := Comment{ID: uuid.New(), PageID: pageID, ParentID: parentID, Body: body, CreatedBy: ""}
	err := s.Pool.QueryRow(ctx, `INSERT INTO comments(id,page_id,parent_id,body,created_by) VALUES($1,$2,$3,$4,$5) RETURNING created_at`, c.ID, pageID, parentID, body, userID).Scan(&c.CreatedAt)
	if err != nil {
		return Comment{}, err
	}
	u, _ := s.UserByID(ctx, userID)
	c.CreatedBy = u.DisplayName
	return c, nil
}
