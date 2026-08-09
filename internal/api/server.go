package api

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/hkjang/kanvas/internal/auth"
	"github.com/hkjang/kanvas/internal/buildinfo"
	"github.com/hkjang/kanvas/internal/mcp"
	"github.com/hkjang/kanvas/internal/migration"
	"github.com/hkjang/kanvas/internal/security"
	"github.com/hkjang/kanvas/internal/store"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Server struct {
	Store         *store.Store
	Auth          *auth.Manager
	Migration     *migration.Service
	MCP           *mcp.Handler
	Vault         *security.Vault
	ConfluenceDSN string
	Static        http.Handler
	Logger        *slog.Logger
	StartedAt     time.Time
	loginMu       sync.Mutex
	loginAttempts map[string][]time.Time
}

func New(st *store.Store, authManager *auth.Manager, migrationService *migration.Service, vault *security.Vault, confluenceDSN string, static http.Handler, logger *slog.Logger) http.Handler {
	s := &Server{Store: st, Auth: authManager, Migration: migrationService, MCP: &mcp.Handler{Store: st}, Vault: vault, ConfluenceDSN: confluenceDSN, Static: static, Logger: logger, StartedAt: time.Now().UTC(), loginAttempts: map[string][]time.Time{}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /readyz", s.ready)
	mux.HandleFunc("GET /api/v1/version", s.version)
	mux.HandleFunc("GET /api/v1/auth/config", s.authConfig)
	mux.HandleFunc("POST /api/v1/auth/login", s.localLogin)
	mux.HandleFunc("POST /api/v1/auth/logout", s.logout)
	mux.HandleFunc("GET /api/v1/auth/oidc/login", s.oidcLogin)
	mux.HandleFunc("GET /api/v1/auth/oidc/callback", s.oidcCallback)
	mux.HandleFunc("GET /api/v1/me", s.me)
	mux.HandleFunc("GET /api/v1/spaces", s.spaces)
	mux.HandleFunc("POST /api/v1/spaces", s.spaces)
	mux.HandleFunc("GET /api/v1/spaces/{spaceID}/pages", s.spacePages)
	mux.HandleFunc("POST /api/v1/spaces/{spaceID}/pages", s.spacePages)
	mux.HandleFunc("GET /api/v1/pages/{pageID}", s.page)
	mux.HandleFunc("PUT /api/v1/pages/{pageID}", s.page)
	mux.HandleFunc("GET /api/v1/pages/{pageID}/versions", s.pageVersions)
	mux.HandleFunc("GET /api/v1/pages/{pageID}/comments", s.comments)
	mux.HandleFunc("POST /api/v1/pages/{pageID}/comments", s.comments)
	mux.HandleFunc("GET /api/v1/search", s.search)
	mux.HandleFunc("GET /api/v1/personal/preferences", s.preferences)
	mux.HandleFunc("PUT /api/v1/personal/preferences", s.preferences)
	mux.HandleFunc("GET /api/v1/personal/api-keys", s.apiKeys)
	mux.HandleFunc("POST /api/v1/personal/api-keys", s.apiKeys)
	mux.HandleFunc("POST /api/v1/personal/api-keys/{keyID}/rotate", s.rotateAPIKey)
	mux.HandleFunc("DELETE /api/v1/personal/api-keys/{keyID}", s.revokeAPIKey)
	mux.HandleFunc("GET /api/v1/admin/settings", s.adminSettings)
	mux.HandleFunc("PUT /api/v1/admin/settings", s.adminSettings)
	mux.HandleFunc("GET /api/v1/admin/overview", s.adminOverview)
	mux.HandleFunc("GET /api/v1/admin/users", s.adminUsers)
	mux.HandleFunc("PATCH /api/v1/admin/users/{userID}", s.updateAdminUser)
	mux.HandleFunc("GET /api/v1/admin/groups", s.adminGroups)
	mux.HandleFunc("POST /api/v1/admin/groups", s.adminGroups)
	mux.HandleFunc("GET /api/v1/admin/groups/{groupID}/members", s.adminGroupMembers)
	mux.HandleFunc("POST /api/v1/admin/groups/{groupID}/members", s.adminGroupMembers)
	mux.HandleFunc("DELETE /api/v1/admin/groups/{groupID}/members/{userID}", s.removeAdminGroupMember)
	mux.HandleFunc("GET /api/v1/admin/spaces", s.adminSpaces)
	mux.HandleFunc("PATCH /api/v1/admin/spaces/{spaceID}", s.updateAdminSpace)
	mux.HandleFunc("GET /api/v1/admin/oidc", s.adminOIDC)
	mux.HandleFunc("PUT /api/v1/admin/oidc", s.adminOIDC)
	mux.HandleFunc("POST /api/v1/admin/connections/postgres/test", s.testPostgres)
	mux.HandleFunc("GET /api/v1/admin/migration", s.adminMigration)
	mux.HandleFunc("POST /api/v1/admin/migration/discovery", s.runDiscovery)
	mux.HandleFunc("POST /api/v1/admin/migration/snapshot", s.startSnapshot)
	mux.HandleFunc("POST /api/v1/admin/migration/reconciliation", s.startReconciliation)
	mux.HandleFunc("POST /api/v1/admin/migration/transition", s.migrationTransition)
	mux.HandleFunc("GET /api/v1/admin/migration/jobs", s.migrationJobs)
	mux.HandleFunc("GET /api/v1/admin/migration/jobs/{jobID}", s.migrationJob)
	mux.HandleFunc("POST /api/v1/admin/migration/jobs/{jobID}/cancel", s.cancelMigrationJob)
	mux.HandleFunc("POST /api/v1/admin/migration/jobs/{jobID}/resume", s.resumeMigrationJob)
	mux.HandleFunc("GET /api/v1/admin/migration/jobs/{jobID}/items", s.migrationJobItems)
	mux.HandleFunc("GET /api/v1/admin/migration/macros", s.migrationMacros)
	mux.HandleFunc("GET /api/v1/admin/migration/unsupported", s.unsupportedContent)
	mux.HandleFunc("PATCH /api/v1/admin/migration/unsupported/{itemID}", s.decideUnsupportedContent)
	mux.HandleFunc("POST /api/v1/admin/migration/unsupported/bulk", s.bulkDecideUnsupportedContent)
	mux.HandleFunc("GET /api/v1/admin/audit", s.auditEvents)
	mux.HandleFunc("GET /api/v1/admin/status", s.adminStatus)
	mux.HandleFunc("GET /api/openapi.yaml", s.openAPI)
	mux.Handle("POST /mcp", s.MCP)
	mux.Handle("/", static)
	return s.recoverer(s.securityHeaders(authManager.Middleware(mux)))
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "service": "Kanvas", "version": buildinfo.Version})
}
func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.Store.Health(ctx); err != nil {
		writeError(w, http.StatusServiceUnavailable, "database is not ready")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ready"})
}
func (s *Server) version(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, buildinfo.Current())
}

func (s *Server) authConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.Auth.OIDCSettings(r.Context())
	if err != nil {
		s.Logger.Warn("load OIDC config", "error", err)
	}
	product := "Kanvas"
	if raw, _, settingErr := s.Store.Setting(r.Context(), "site.name"); settingErr == nil {
		_ = json.Unmarshal(raw, &product)
		if strings.TrimSpace(product) == "" {
			product = "Kanvas"
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"oidcEnabled": err == nil && cfg.Enabled, "localLoginEnabled": true, "product": product})
}

func (s *Server) localLogin(w http.ResponseWriter, r *http.Request) {
	if !s.allowLogin(r) {
		writeError(w, http.StatusTooManyRequests, "too many login attempts; try again later")
		return
	}
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	u, err := s.Store.AuthenticateLocal(r.Context(), strings.TrimSpace(in.Username), in.Password)
	if err != nil {
		s.recordLoginFailure(r)
		s.Store.Audit(r.Context(), nil, "LOGIN_FAILED", "USER", strings.TrimSpace(in.Username), r.RemoteAddr, r.UserAgent(), map[string]any{})
		writeError(w, http.StatusUnauthorized, "invalid username or password")
		return
	}
	if err = s.Auth.NewSession(w, r, u); err != nil {
		writeError(w, http.StatusInternalServerError, "could not create session")
		return
	}
	s.clearLoginFailures(r)
	s.Store.Audit(r.Context(), &u.ID, "LOGIN_LOCAL", "USER", u.ID.String(), r.RemoteAddr, r.UserAgent(), map[string]any{})
	writeJSON(w, http.StatusOK, map[string]any{"user": u})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	id, ok := s.authorize(w, r, "", "")
	if !ok {
		return
	}
	s.Auth.Logout(w, r)
	s.Store.Audit(r.Context(), &id.User.ID, "LOGOUT", "USER", id.User.ID.String(), r.RemoteAddr, r.UserAgent(), map[string]any{})
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) oidcLogin(w http.ResponseWriter, r *http.Request) {
	if err := s.Auth.OIDCLogin(w, r); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
	}
}
func (s *Server) oidcCallback(w http.ResponseWriter, r *http.Request) {
	if err := s.Auth.OIDCCallback(w, r); err != nil {
		s.Logger.Warn("OIDC callback failed", "error", err)
		writeError(w, http.StatusUnauthorized, "OIDC login failed")
	}
}
func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	id, ok := s.authorize(w, r, "", "")
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": id.User, "csrfToken": id.CSRFToken, "authMethod": id.AuthMethod})
}

func (s *Server) spaces(w http.ResponseWriter, r *http.Request) {
	id, ok := s.authorize(w, r, "", mapMethodScope(r.Method))
	if !ok {
		return
	}
	if r.Method == "GET" {
		v, err := s.Store.ListSpaces(r.Context(), id.User.ID)
		respond(w, v, err)
		return
	}
	var in struct {
		Key         string `json:"key"`
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	if strings.TrimSpace(in.Key) == "" || strings.TrimSpace(in.Name) == "" {
		writeError(w, http.StatusBadRequest, "key and name are required")
		return
	}
	v, err := s.Store.CreateSpace(r.Context(), id.User.ID, in.Key, in.Name, in.Description)
	if err == nil {
		s.Store.Audit(r.Context(), &id.User.ID, "SPACE_CREATE", "SPACE", v.ID.String(), r.RemoteAddr, r.UserAgent(), map[string]any{"key": v.Key})
	}
	respondStatus(w, http.StatusCreated, v, err)
}

func (s *Server) spacePages(w http.ResponseWriter, r *http.Request) {
	id, ok := s.authorize(w, r, "", mapMethodScope(r.Method))
	if !ok {
		return
	}
	spaceID, err := uuid.Parse(r.PathValue("spaceID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid space ID")
		return
	}
	if r.Method == "GET" {
		v, err := s.Store.PagesInSpace(r.Context(), id.User.ID, spaceID)
		respond(w, v, err)
		return
	}
	var in struct {
		ParentID       *uuid.UUID      `json:"parentId"`
		Title          string          `json:"title"`
		EditorDocument json.RawMessage `json:"editorDocument"`
		RenderedText   string          `json:"renderedText"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	if strings.TrimSpace(in.Title) == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}
	v, err := s.Store.CreatePage(r.Context(), id.User.ID, spaceID, in.ParentID, in.Title, in.EditorDocument, in.RenderedText)
	if err == nil {
		s.Store.Audit(r.Context(), &id.User.ID, "PAGE_CREATE", "PAGE", v.ID.String(), r.RemoteAddr, r.UserAgent(), map[string]any{"title": v.Title})
	}
	respondStatus(w, http.StatusCreated, v, err)
}

func (s *Server) page(w http.ResponseWriter, r *http.Request) {
	id, ok := s.authorize(w, r, "", mapMethodScope(r.Method))
	if !ok {
		return
	}
	pageID, err := uuid.Parse(r.PathValue("pageID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid page ID")
		return
	}
	if r.Method == "GET" {
		v, err := s.Store.PageByID(r.Context(), id.User.ID, pageID)
		respond(w, v, err)
		return
	}
	var in struct {
		Title          string          `json:"title"`
		EditorDocument json.RawMessage `json:"editorDocument"`
		RenderedText   string          `json:"renderedText"`
		ChangeMessage  string          `json:"changeMessage"`
		Version        int             `json:"version"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	v, err := s.Store.UpdatePage(r.Context(), id.User.ID, pageID, in.Title, in.EditorDocument, in.RenderedText, in.ChangeMessage, in.Version)
	if err == nil {
		s.Store.Audit(r.Context(), &id.User.ID, "PAGE_EDIT", "PAGE", pageID.String(), r.RemoteAddr, r.UserAgent(), map[string]any{"version": v.CurrentVersion})
	}
	respond(w, v, err)
}

func (s *Server) pageVersions(w http.ResponseWriter, r *http.Request) {
	id, ok := s.authorize(w, r, "", "wiki:read")
	if !ok {
		return
	}
	pageID, err := uuid.Parse(r.PathValue("pageID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid page ID")
		return
	}
	allowed, err := s.Store.CanPage(r.Context(), id.User.ID, pageID, "VIEW")
	if err != nil || !allowed {
		writeError(w, http.StatusNotFound, "page not found")
		return
	}
	v, err := s.Store.PageVersions(r.Context(), id.User.ID, pageID)
	respond(w, v, err)
}
func (s *Server) comments(w http.ResponseWriter, r *http.Request) {
	id, ok := s.authorize(w, r, "", mapMethodScope(r.Method))
	if !ok {
		return
	}
	pageID, err := uuid.Parse(r.PathValue("pageID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid page ID")
		return
	}
	allowed, err := s.Store.CanPage(r.Context(), id.User.ID, pageID, map[bool]string{true: "VIEW", false: "COMMENT"}[r.Method == "GET"])
	if err != nil || !allowed {
		writeError(w, http.StatusForbidden, "permission denied")
		return
	}
	if r.Method == "GET" {
		v, err := s.Store.Comments(r.Context(), id.User.ID, pageID)
		respond(w, v, err)
		return
	}
	var in struct {
		ParentID *uuid.UUID `json:"parentId"`
		Body     string     `json:"body"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	if strings.TrimSpace(in.Body) == "" {
		writeError(w, http.StatusBadRequest, "comment body is required")
		return
	}
	v, err := s.Store.AddComment(r.Context(), id.User.ID, pageID, in.ParentID, in.Body)
	if err == nil {
		s.Store.Audit(r.Context(), &id.User.ID, "COMMENT_CREATE", "PAGE", pageID.String(), r.RemoteAddr, r.UserAgent(), map[string]any{"commentId": v.ID})
	}
	respondStatus(w, http.StatusCreated, v, err)
}
func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	id, ok := s.authorize(w, r, "", "wiki:read")
	if !ok {
		return
	}
	v, err := s.Store.SearchPages(r.Context(), id.User.ID, strings.TrimSpace(r.URL.Query().Get("q")), 30)
	respond(w, v, err)
}

func (s *Server) preferences(w http.ResponseWriter, r *http.Request) {
	id, ok := s.authorize(w, r, "", mapMethodScope(r.Method))
	if !ok {
		return
	}
	if r.Method == "GET" {
		v, err := s.Store.Preferences(r.Context(), id.User.ID)
		respond(w, v, err)
		return
	}
	var in store.Preferences
	if !decodeJSON(w, r, &in) {
		return
	}
	if in.Locale == "" {
		in.Locale = "ko-KR"
	}
	if in.Theme != "light" && in.Theme != "dark" && in.Theme != "system" {
		writeError(w, http.StatusBadRequest, "invalid theme")
		return
	}
	if err := s.Store.PutPreferences(r.Context(), id.User.ID, in); err != nil {
		respond(w, nil, err)
		return
	}
	s.Store.Audit(r.Context(), &id.User.ID, "PERSONAL_SETTINGS_UPDATE", "USER", id.User.ID.String(), r.RemoteAddr, r.UserAgent(), map[string]any{})
	respond(w, in, nil)
}

type keyInput struct {
	Name      string     `json:"name"`
	Scopes    []string   `json:"scopes"`
	ExpiresAt *time.Time `json:"expiresAt"`
}

func (s *Server) apiKeys(w http.ResponseWriter, r *http.Request) {
	id, ok := s.authorize(w, r, "", mapMethodScope(r.Method))
	if !ok {
		return
	}
	if r.Method == "GET" {
		v, err := s.Store.APIKeys(r.Context(), id.User.ID)
		respond(w, v, err)
		return
	}
	var in keyInput
	if !decodeJSON(w, r, &in) {
		return
	}
	if strings.TrimSpace(in.Name) == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if !validScopes(in.Scopes) {
		writeError(w, http.StatusBadRequest, "scopes must be wiki:read and/or wiki:write")
		return
	}
	token, prefix, hash, err := newAPIKey()
	if err != nil {
		respond(w, nil, err)
		return
	}
	v, err := s.Store.CreateAPIKey(r.Context(), id.User.ID, in.Name, prefix, hash, in.Scopes, in.ExpiresAt, nil)
	if err == nil {
		s.Store.Audit(r.Context(), &id.User.ID, "API_KEY_CREATE", "API_KEY", v.ID.String(), r.RemoteAddr, r.UserAgent(), map[string]any{"prefix": prefix})
	}
	respondStatus(w, http.StatusCreated, map[string]any{"key": v, "token": token}, err)
}
func (s *Server) rotateAPIKey(w http.ResponseWriter, r *http.Request) {
	id, ok := s.authorize(w, r, "", "wiki:write")
	if !ok {
		return
	}
	keyID, err := uuid.Parse(r.PathValue("keyID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid key ID")
		return
	}
	old, err := s.Store.APIKeyByID(r.Context(), id.User.ID, keyID)
	if err != nil {
		respond(w, nil, err)
		return
	}
	token, prefix, hash, err := newAPIKey()
	if err != nil {
		respond(w, nil, err)
		return
	}
	v, err := s.Store.RotateAPIKey(r.Context(), id.User.ID, keyID, old.Name, prefix, hash, old.Scopes, old.ExpiresAt)
	if err == nil {
		s.Store.Audit(r.Context(), &id.User.ID, "API_KEY_ROTATE", "API_KEY", v.ID.String(), r.RemoteAddr, r.UserAgent(), map[string]any{"rotatedFrom": keyID, "prefix": prefix})
	}
	respondStatus(w, http.StatusCreated, map[string]any{"key": v, "token": token}, err)
}
func (s *Server) revokeAPIKey(w http.ResponseWriter, r *http.Request) {
	id, ok := s.authorize(w, r, "", "wiki:write")
	if !ok {
		return
	}
	keyID, err := uuid.Parse(r.PathValue("keyID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid key ID")
		return
	}
	err = s.Store.RevokeAPIKey(r.Context(), id.User.ID, keyID)
	if err == nil {
		s.Store.Audit(r.Context(), &id.User.ID, "API_KEY_REVOKE", "API_KEY", keyID.String(), r.RemoteAddr, r.UserAgent(), map[string]any{})
		w.WriteHeader(http.StatusNoContent)
		return
	}
	respond(w, nil, err)
}

func (s *Server) adminSettings(w http.ResponseWriter, r *http.Request) {
	id, ok := s.authorize(w, r, "ADMIN", "*")
	if !ok {
		return
	}
	if r.Method == "GET" {
		settings, err := s.Store.Settings(r.Context())
		if err != nil {
			respond(w, nil, err)
			return
		}
		for i := range settings {
			if settings[i].Secret {
				settings[i].Value = json.RawMessage(`"********"`)
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"settings": settings, "environment": map[string]any{"postgres": connectionFingerprint(s.Store.Pool.Config().ConnString()), "confluence": connectionFingerprint(s.ConfluenceDSN)}})
		return
	}
	var in struct {
		Key         string `json:"key"`
		Value       any    `json:"value"`
		Description string `json:"description"`
		Secret      bool   `json:"secret"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	allowed := map[string]bool{"site.name": true, "site.base_url": true, "attachments.root": true, "attachments.copy_threads": true, "attachments.hash_algorithm": true, "migration.batch_size": true, "migration.parallelism": true, "migration.max_throughput": true, "migration.attachment_root": true, "security.session_hours": true, "search.language": true, "backup.status": true}
	if !allowed[in.Key] {
		writeError(w, http.StatusBadRequest, "setting key is not managed by this endpoint")
		return
	}
	if err := validateManagedSetting(in.Key, in.Value); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	value := in.Value
	if in.Secret {
		plain, ok := in.Value.(string)
		if !ok {
			writeError(w, http.StatusBadRequest, "secret value must be a string")
			return
		}
		encrypted, err := s.Vault.Encrypt(plain)
		if err != nil {
			respond(w, nil, err)
			return
		}
		value = encrypted
	}
	err := s.Store.PutSetting(r.Context(), in.Key, value, in.Secret, in.Description, id.User.ID)
	if err == nil {
		s.Store.Audit(r.Context(), &id.User.ID, "ADMIN_SETTING_UPDATE", "SETTING", in.Key, r.RemoteAddr, r.UserAgent(), map[string]any{"secret": in.Secret})
	}
	respond(w, map[string]any{"saved": err == nil}, err)
}

func (s *Server) adminOIDC(w http.ResponseWriter, r *http.Request) {
	id, ok := s.authorize(w, r, "ADMIN", "*")
	if !ok {
		return
	}
	if r.Method == "GET" {
		cfg, err := s.Auth.OIDCSettings(r.Context())
		if err != nil {
			respond(w, nil, err)
			return
		}
		has := cfg.ClientSecret != ""
		cfg.ClientSecret = ""
		writeJSON(w, http.StatusOK, map[string]any{"config": cfg, "hasClientSecret": has})
		return
	}
	var in struct {
		Config             auth.OIDCSettings `json:"config"`
		KeepExistingSecret bool              `json:"keepExistingSecret"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	err := s.Auth.SaveOIDCSettings(r.Context(), id.User.ID, in.Config, in.KeepExistingSecret)
	if err == nil {
		s.Store.Audit(r.Context(), &id.User.ID, "OIDC_SETTING_UPDATE", "SETTING", "auth.oidc", r.RemoteAddr, r.UserAgent(), map[string]any{"enabled": in.Config.Enabled, "issuer": in.Config.Issuer})
	}
	respond(w, map[string]any{"saved": err == nil}, err)
}

func (s *Server) testPostgres(w http.ResponseWriter, r *http.Request) {
	_, ok := s.authorize(w, r, "ADMIN", "*")
	if !ok {
		return
	}
	var in struct {
		DSN string `json:"dsn"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	cfg, err := pgxpool.ParseConfig(in.DSN)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid PostgreSQL DSN")
		return
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		writeError(w, http.StatusBadRequest, "could not create PostgreSQL connection")
		return
	}
	defer pool.Close()
	var version string
	err = pool.QueryRow(ctx, `SHOW server_version`).Scan(&version)
	if err != nil {
		writeError(w, http.StatusBadRequest, "PostgreSQL connection failed")
		return
	}
	var canCreate bool
	_ = pool.QueryRow(ctx, `SELECT has_schema_privilege(current_user,current_schema(),'CREATE')`).Scan(&canCreate)
	writeJSON(w, http.StatusOK, map[string]any{"connected": true, "version": version, "schemaCreate": canCreate})
}

func (s *Server) adminMigration(w http.ResponseWriter, r *http.Request) {
	_, ok := s.authorize(w, r, "ADMIN", "*")
	if !ok {
		return
	}
	v, err := s.Migration.Dashboard(r.Context())
	respond(w, v, err)
}
func (s *Server) runDiscovery(w http.ResponseWriter, r *http.Request) {
	id, ok := s.authorize(w, r, "ADMIN", "*")
	if !ok {
		return
	}
	dsn, err := s.effectiveConfluenceDSN(r.Context())
	if err != nil {
		respond(w, nil, err)
		return
	}
	v, err := s.Migration.Discover(r.Context(), dsn, id.User.ID)
	if err != nil {
		s.Store.Audit(r.Context(), &id.User.ID, "SCHEMA_DISCOVERY_FAILED", "MIGRATION", "", r.RemoteAddr, r.UserAgent(), map[string]any{"error": err.Error()})
	}
	respond(w, v, err)
}
func (s *Server) startSnapshot(w http.ResponseWriter, r *http.Request) {
	id, ok := s.authorize(w, r, "ADMIN", "*")
	if !ok {
		return
	}
	options := migration.DefaultSnapshotOptions()
	if r.ContentLength > 0 && !decodeJSON(w, r, &options) {
		return
	}
	dsn, err := s.effectiveConfluenceDSN(r.Context())
	if err != nil {
		respond(w, nil, err)
		return
	}
	job, err := s.Migration.StartSnapshot(r.Context(), dsn, id.User.ID, options)
	respondStatus(w, http.StatusAccepted, job, err)
}
func (s *Server) migrationJobs(w http.ResponseWriter, r *http.Request) {
	_, ok := s.authorize(w, r, "ADMIN", "*")
	if !ok {
		return
	}
	jobs, err := s.Migration.Jobs(r.Context(), 100)
	respond(w, jobs, err)
}
func (s *Server) migrationJob(w http.ResponseWriter, r *http.Request) {
	_, ok := s.authorize(w, r, "ADMIN", "*")
	if !ok {
		return
	}
	jobID, err := uuid.Parse(r.PathValue("jobID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid job ID")
		return
	}
	job, err := s.Migration.Job(r.Context(), jobID)
	respond(w, job, err)
}
func (s *Server) cancelMigrationJob(w http.ResponseWriter, r *http.Request) {
	id, ok := s.authorize(w, r, "ADMIN", "*")
	if !ok {
		return
	}
	jobID, err := uuid.Parse(r.PathValue("jobID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid job ID")
		return
	}
	err = s.Migration.CancelJob(r.Context(), jobID, id.User.ID)
	if err == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	respond(w, nil, err)
}
func (s *Server) resumeMigrationJob(w http.ResponseWriter, r *http.Request) {
	id, ok := s.authorize(w, r, "ADMIN", "*")
	if !ok {
		return
	}
	jobID, err := uuid.Parse(r.PathValue("jobID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid job ID")
		return
	}
	dsn, err := s.effectiveConfluenceDSN(r.Context())
	if err != nil {
		respond(w, nil, err)
		return
	}
	job, err := s.Migration.ResumeSnapshot(r.Context(), dsn, jobID, id.User.ID)
	respondStatus(w, http.StatusAccepted, job, err)
}
func (s *Server) migrationJobItems(w http.ResponseWriter, r *http.Request) {
	_, ok := s.authorize(w, r, "ADMIN", "*")
	if !ok {
		return
	}
	jobID, err := uuid.Parse(r.PathValue("jobID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid job ID")
		return
	}
	items, err := s.Migration.JobItems(r.Context(), jobID, r.URL.Query().Get("status"), 500)
	respond(w, items, err)
}
func (s *Server) migrationMacros(w http.ResponseWriter, r *http.Request) {
	_, ok := s.authorize(w, r, "ADMIN", "*")
	if !ok {
		return
	}
	rows, err := s.Store.Pool.Query(r.Context(), `SELECT macro_name,support_level,page_count,occurrence_count,conversion_rate,last_seen_at,details FROM macro_compatibility ORDER BY occurrence_count DESC,macro_name`)
	if err != nil {
		respond(w, nil, err)
		return
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		var name, level string
		var pages, occurrences int64
		var rate float64
		var seen time.Time
		var details json.RawMessage
		if err = rows.Scan(&name, &level, &pages, &occurrences, &rate, &seen, &details); err != nil {
			respond(w, nil, err)
			return
		}
		out = append(out, map[string]any{"name": name, "supportLevel": level, "pageCount": pages, "occurrenceCount": occurrences, "conversionRate": rate, "lastSeenAt": seen, "details": details})
	}
	respond(w, out, rows.Err())
}
func (s *Server) migrationTransition(w http.ResponseWriter, r *http.Request) {
	id, ok := s.authorize(w, r, "ADMIN", "*")
	if !ok {
		return
	}
	var in struct {
		Target string `json:"target"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	err := s.Migration.Transition(r.Context(), in.Target, id.User.ID)
	respond(w, map[string]any{"transitioned": err == nil}, err)
}

func (s *Server) auditEvents(w http.ResponseWriter, r *http.Request) {
	_, ok := s.authorize(w, r, "ADMIN", "*")
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 || limit > 500 {
		limit = 100
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	action := strings.TrimSpace(r.URL.Query().Get("action"))
	rows, err := s.Store.Pool.Query(r.Context(), `SELECT a.id,coalesce(u.display_name,'System'),a.action,a.resource_type,a.resource_id,a.remote_addr,a.detail,a.created_at
		FROM audit_events a LEFT JOIN users u ON u.id=a.actor_id
		WHERE ($2='' OR a.action ILIKE '%'||$2||'%' OR a.resource_type ILIKE '%'||$2||'%' OR a.resource_id ILIKE '%'||$2||'%' OR coalesce(u.display_name,'System') ILIKE '%'||$2||'%')
		AND ($3='' OR a.action=$3) ORDER BY a.created_at DESC LIMIT $1`, limit, query, action)
	if err != nil {
		respond(w, nil, err)
		return
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		var id uuid.UUID
		var actor, action, rt, rid, remote string
		var detail json.RawMessage
		var at time.Time
		if err = rows.Scan(&id, &actor, &action, &rt, &rid, &remote, &detail, &at); err != nil {
			respond(w, nil, err)
			return
		}
		out = append(out, map[string]any{"id": id, "actor": actor, "action": action, "resourceType": rt, "resourceId": rid, "remoteAddr": remote, "detail": detail, "createdAt": at})
	}
	respond(w, out, rows.Err())
}
func (s *Server) adminStatus(w http.ResponseWriter, r *http.Request) {
	_, ok := s.authorize(w, r, "ADMIN", "*")
	if !ok {
		return
	}
	stats := s.Store.Pool.Stat()
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	writeJSON(w, http.StatusOK, map[string]any{
		"service":  buildinfo.Current(),
		"database": map[string]any{"status": "connected", "totalConnections": stats.TotalConns(), "idleConnections": stats.IdleConns(), "acquiredConnections": stats.AcquiredConns(), "maxConnections": stats.MaxConns()},
		"runtime":  map[string]any{"time": time.Now().UTC(), "startedAt": s.StartedAt, "uptimeSeconds": int64(time.Since(s.StartedAt).Seconds()), "goroutines": runtime.NumGoroutine(), "memoryAllocBytes": memory.Alloc, "memorySystemBytes": memory.Sys, "goVersion": runtime.Version()},
	})
}

func (s *Server) openAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	_, _ = w.Write([]byte(openAPISpec))
}

func (s *Server) effectiveConfluenceDSN(ctx context.Context) (string, error) {
	raw, secret, err := s.Store.Setting(ctx, "source.confluence_dsn")
	if errors.Is(err, pgx.ErrNoRows) {
		return s.ConfluenceDSN, nil
	}
	if err != nil {
		return "", err
	}
	var value string
	if err = json.Unmarshal(raw, &value); err != nil {
		return "", err
	}
	if secret {
		return s.Vault.Decrypt(value)
	}
	return value, nil
}

func (s *Server) authorize(w http.ResponseWriter, r *http.Request, role, scope string) (auth.Identity, bool) {
	id, ok := auth.IdentityFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return id, false
	}
	if role != "" && id.User.Role != role {
		writeError(w, http.StatusForbidden, "administrator access required")
		return id, false
	}
	if !safeMethod(r.Method) && id.AuthMethod == "session" && r.Header.Get("X-CSRF-Token") != id.CSRFToken {
		writeError(w, http.StatusForbidden, "invalid CSRF token")
		return id, false
	}
	if id.AuthMethod == "api_key" && scope != "" {
		if scope == "*" || (!contains(id.Scopes, scope) && !contains(id.Scopes, "*")) {
			writeError(w, http.StatusForbidden, "API key scope denied")
			return id, false
		}
	}
	return id, true
}
func mapMethodScope(method string) string {
	if safeMethod(method) {
		return "wiki:read"
	}
	return "wiki:write"
}
func safeMethod(method string) bool {
	return method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data: blob:; style-src 'self'; script-src 'self'; connect-src 'self' ws: wss:; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
		next.ServeHTTP(w, r)
	})
}
func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				s.Logger.Error("request panic", "panic", v, "path", r.URL.Path)
				writeError(w, http.StatusInternalServerError, "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (s *Server) allowLogin(r *http.Request) bool {
	s.loginMu.Lock()
	defer s.loginMu.Unlock()
	key := clientKey(r)
	cutoff := time.Now().Add(-15 * time.Minute)
	items := s.loginAttempts[key]
	kept := items[:0]
	for _, x := range items {
		if x.After(cutoff) {
			kept = append(kept, x)
		}
	}
	s.loginAttempts[key] = kept
	return len(kept) < 10
}
func (s *Server) recordLoginFailure(r *http.Request) {
	s.loginMu.Lock()
	defer s.loginMu.Unlock()
	key := clientKey(r)
	s.loginAttempts[key] = append(s.loginAttempts[key], time.Now())
}
func (s *Server) clearLoginFailures(r *http.Request) {
	s.loginMu.Lock()
	defer s.loginMu.Unlock()
	delete(s.loginAttempts, clientKey(r))
}
func clientKey(r *http.Request) string {
	if x := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-For"), ",")[0]); x != "" {
		return x
	}
	return strings.Split(r.RemoteAddr, ":")[0]
}

func newAPIKey() (token, prefix, hash string, err error) {
	raw, err := security.RandomToken(32)
	if err != nil {
		return "", "", "", err
	}
	token = "knv_" + raw
	prefix = token
	if len(prefix) > 12 {
		prefix = prefix[:12]
	}
	return token, prefix, security.HashToken(token), nil
}
func validScopes(scopes []string) bool {
	if len(scopes) == 0 {
		return false
	}
	for _, v := range scopes {
		if v != "wiki:read" && v != "wiki:write" {
			return false
		}
	}
	return true
}
func contains(values []string, value string) bool {
	for _, x := range values {
		if x == value {
			return true
		}
	}
	return false
}
func connectionFingerprint(dsn string) map[string]any {
	if strings.TrimSpace(dsn) == "" {
		return map[string]any{"configured": false}
	}
	sum := sha256.Sum256([]byte(dsn))
	return map[string]any{"configured": true, "fingerprint": fmt.Sprintf("%x", sum[:6])}
}

func validateManagedSetting(key string, value any) error {
	stringValue, isString := value.(string)
	numberValue, isNumber := value.(float64)
	switch key {
	case "site.name":
		if !isString || strings.TrimSpace(stringValue) == "" || len([]rune(stringValue)) > 80 {
			return errors.New("site name must contain 1 to 80 characters")
		}
	case "site.base_url":
		if !isString {
			return errors.New("site base URL must be a string")
		}
		if strings.TrimSpace(stringValue) == "" {
			return nil
		}
		parsed, err := url.Parse(strings.TrimSpace(stringValue))
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
			return errors.New("site base URL must be an absolute HTTP(S) URL without credentials")
		}
	case "security.session_hours":
		if !isNumber || math.Trunc(numberValue) != numberValue || numberValue < 1 || numberValue > 168 {
			return errors.New("session duration must be an integer from 1 to 168 hours")
		}
	case "migration.batch_size":
		if !isNumber || math.Trunc(numberValue) != numberValue || numberValue < 10 || numberValue > 5000 {
			return errors.New("migration batch size must be an integer from 10 to 5000")
		}
	case "migration.parallelism", "attachments.copy_threads":
		if !isNumber || math.Trunc(numberValue) != numberValue || numberValue < 1 || numberValue > 32 {
			return errors.New("worker count must be an integer from 1 to 32")
		}
	}
	return nil
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON request: "+err.Error())
		return false
	}
	return true
}
func respond(w http.ResponseWriter, value any, err error) {
	respondStatus(w, http.StatusOK, value, err)
}
func respondStatus(w http.ResponseWriter, status int, value any, err error) {
	if err == nil {
		writeJSON(w, status, value)
		return
	}
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "resource not found")
		return
	}
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) && postgresError.Code == "23505" {
		writeError(w, http.StatusConflict, "resource already exists")
		return
	}
	if strings.Contains(err.Error(), "version conflict") {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if strings.Contains(err.Error(), "already active") {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if strings.Contains(err.Error(), "last active administrator") || strings.Contains(err.Error(), "your own administrator account") {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if strings.Contains(err.Error(), "must be USER or ADMIN") || strings.Contains(err.Error(), "must be ACTIVE or DISABLED") || strings.Contains(err.Error(), "status must be ACTIVE or ARCHIVED") || strings.Contains(err.Error(), "group name is required") || strings.Contains(err.Error(), "unsupported content status") || strings.Contains(err.Error(), "unsupported content decision") || strings.Contains(err.Error(), "resolution is required") || strings.Contains(err.Error(), "resolution must be") {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.Contains(err.Error(), "not implemented in this release") {
		writeError(w, http.StatusNotImplemented, err.Error())
		return
	}
	if strings.Contains(err.Error(), "must complete before snapshot") {
		writeError(w, http.StatusPreconditionFailed, err.Error())
		return
	}
	if strings.Contains(err.Error(), "completed snapshot is required") {
		writeError(w, http.StatusPreconditionFailed, err.Error())
		return
	}
	if strings.Contains(err.Error(), "permission denied") || strings.Contains(err.Error(), "gate rejected") || strings.Contains(err.Error(), "invalid migration transition") {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}
	writeError(w, http.StatusInternalServerError, "request failed")
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{"error": message})
}
