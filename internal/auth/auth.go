package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/google/uuid"
	"github.com/hkjang/kanvas/internal/security"
	"github.com/hkjang/kanvas/internal/store"
	"github.com/jackc/pgx/v5"
	"golang.org/x/oauth2"
)

const SessionCookie = "kanvas_session"

type identityKey struct{}

type Identity struct {
	User       store.User
	AuthMethod string
	CSRFToken  string
	Scopes     []string
}

type Manager struct {
	Store *store.Store
	Vault *security.Vault
}

type OIDCSettings struct {
	Enabled       bool   `json:"enabled"`
	Issuer        string `json:"issuer"`
	ClientID      string `json:"clientId"`
	ClientSecret  string `json:"clientSecret,omitempty"`
	GroupsClaim   string `json:"groupsClaim"`
	AdminGroup    string `json:"adminGroup"`
	AutoProvision bool   `json:"autoProvision"`
}

func WithIdentity(ctx context.Context, id Identity) context.Context {
	return context.WithValue(ctx, identityKey{}, id)
}

func IdentityFrom(ctx context.Context) (Identity, bool) {
	id, ok := ctx.Value(identityKey{}).(Identity)
	return id, ok
}

func (m *Manager) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if id, ok := m.authenticate(r); ok {
			r = r.WithContext(WithIdentity(r.Context(), id))
		}
		next.ServeHTTP(w, r)
	})
}

func (m *Manager) authenticate(r *http.Request) (Identity, bool) {
	if bearer := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")); bearer != "" && bearer != r.Header.Get("Authorization") {
		u, scopes, err := m.Store.APIKeyUser(r.Context(), security.HashToken(bearer))
		if err == nil {
			return Identity{User: u, AuthMethod: "api_key", Scopes: scopes}, true
		}
	}
	cookie, err := r.Cookie(SessionCookie)
	if err != nil || cookie.Value == "" {
		return Identity{}, false
	}
	u, csrf, err := m.Store.SessionUser(r.Context(), security.HashToken(cookie.Value))
	if err != nil {
		return Identity{}, false
	}
	return Identity{User: u, AuthMethod: "session", CSRFToken: csrf, Scopes: []string{"wiki:read", "wiki:write"}}, true
}

func (m *Manager) NewSession(w http.ResponseWriter, r *http.Request, u store.User) error {
	token, err := security.RandomToken(32)
	if err != nil {
		return err
	}
	csrf, err := security.RandomToken(24)
	if err != nil {
		return err
	}
	sessionHours := 12
	if raw, _, settingErr := m.Store.Setting(r.Context(), "security.session_hours"); settingErr == nil {
		var configured int
		if json.Unmarshal(raw, &configured) == nil && configured >= 1 && configured <= 168 {
			sessionHours = configured
		}
	}
	duration := time.Duration(sessionHours) * time.Hour
	expires := time.Now().Add(duration)
	if err = m.Store.CreateSession(r.Context(), u.ID, security.HashToken(token), csrf, remoteAddr(r), r.UserAgent(), expires); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{Name: SessionCookie, Value: token, Path: "/", HttpOnly: true, Secure: isSecure(r), SameSite: http.SameSiteLaxMode, Expires: expires, MaxAge: int(duration.Seconds())})
	return nil
}

func (m *Manager) Logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(SessionCookie); err == nil {
		_ = m.Store.DeleteSession(r.Context(), security.HashToken(c.Value))
	}
	http.SetCookie(w, &http.Cookie{Name: SessionCookie, Value: "", Path: "/", HttpOnly: true, Secure: isSecure(r), SameSite: http.SameSiteLaxMode, MaxAge: -1, Expires: time.Unix(0, 0)})
}

func (m *Manager) OIDCSettings(ctx context.Context) (OIDCSettings, error) {
	var cfg OIDCSettings
	read := func(key string, target any) error {
		raw, _, err := m.Store.Setting(ctx, key)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		return json.Unmarshal(raw, target)
	}
	_ = read("auth.oidc.enabled", &cfg.Enabled)
	_ = read("auth.oidc.issuer", &cfg.Issuer)
	_ = read("auth.oidc.client_id", &cfg.ClientID)
	var encrypted string
	if err := read("auth.oidc.client_secret", &encrypted); err != nil {
		return cfg, err
	}
	if encrypted != "" {
		plain, err := m.Vault.Decrypt(encrypted)
		if err != nil {
			return cfg, fmt.Errorf("decrypt oidc client secret: %w", err)
		}
		cfg.ClientSecret = plain
	}
	_ = read("auth.oidc.groups_claim", &cfg.GroupsClaim)
	_ = read("auth.oidc.admin_group", &cfg.AdminGroup)
	_ = read("auth.oidc.auto_provision", &cfg.AutoProvision)
	if cfg.GroupsClaim == "" {
		cfg.GroupsClaim = "groups"
	}
	return cfg, nil
}

func (m *Manager) SaveOIDCSettings(ctx context.Context, actor uuid.UUID, cfg OIDCSettings, keepExistingSecret bool) error {
	if cfg.Enabled && (cfg.Issuer == "" || cfg.ClientID == "") {
		return errors.New("issuer and client ID are required when OIDC is enabled")
	}
	values := []struct {
		key         string
		value       any
		secret      bool
		description string
	}{
		{"auth.oidc.enabled", cfg.Enabled, false, "Keycloak/OIDC login enabled"},
		{"auth.oidc.issuer", strings.TrimSuffix(cfg.Issuer, "/"), false, "OIDC issuer URL"},
		{"auth.oidc.client_id", cfg.ClientID, false, "OIDC client ID"},
		{"auth.oidc.groups_claim", cfg.GroupsClaim, false, "OIDC groups claim"},
		{"auth.oidc.admin_group", cfg.AdminGroup, false, "OIDC group mapped to Kanvas admin"},
		{"auth.oidc.auto_provision", cfg.AutoProvision, false, "Create users at first OIDC login"},
	}
	for _, v := range values {
		if err := m.Store.PutSetting(ctx, v.key, v.value, v.secret, v.description, actor); err != nil {
			return err
		}
	}
	if cfg.ClientSecret != "" && !keepExistingSecret {
		encrypted, err := m.Vault.Encrypt(cfg.ClientSecret)
		if err != nil {
			return err
		}
		if err = m.Store.PutSetting(ctx, "auth.oidc.client_secret", encrypted, true, "Encrypted OIDC client secret", actor); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) OIDCLogin(w http.ResponseWriter, r *http.Request) error {
	cfg, provider, oauthCfg, err := m.oidcClient(r)
	_ = provider
	if err != nil {
		return err
	}
	if !cfg.Enabled {
		return errors.New("OIDC is disabled")
	}
	state, err := security.RandomToken(24)
	if err != nil {
		return err
	}
	nonce, err := security.RandomToken(24)
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{Name: "kanvas_oidc_state", Value: state + "." + nonce, Path: "/api/v1/auth/oidc/callback", HttpOnly: true, Secure: isSecure(r), SameSite: http.SameSiteLaxMode, MaxAge: 600})
	http.Redirect(w, r, oauthCfg.AuthCodeURL(state, oidc.Nonce(nonce)), http.StatusFound)
	return nil
}

func (m *Manager) OIDCCallback(w http.ResponseWriter, r *http.Request) error {
	cookie, err := r.Cookie("kanvas_oidc_state")
	if err != nil {
		return errors.New("missing OIDC state cookie")
	}
	parts := strings.SplitN(cookie.Value, ".", 2)
	if len(parts) != 2 || parts[0] != r.URL.Query().Get("state") {
		return errors.New("invalid OIDC state")
	}
	cfg, provider, oauthCfg, err := m.oidcClient(r)
	if err != nil {
		return err
	}
	token, err := oauthCfg.Exchange(r.Context(), r.URL.Query().Get("code"))
	if err != nil {
		return fmt.Errorf("exchange OIDC code: %w", err)
	}
	raw, ok := token.Extra("id_token").(string)
	if !ok {
		return errors.New("OIDC response has no ID token")
	}
	idToken, err := provider.Verifier(&oidc.Config{ClientID: cfg.ClientID}).Verify(r.Context(), raw)
	if err != nil {
		return fmt.Errorf("verify ID token: %w", err)
	}
	var claims struct {
		Subject  string         `json:"sub"`
		Username string         `json:"preferred_username"`
		Name     string         `json:"name"`
		Email    string         `json:"email"`
		Nonce    string         `json:"nonce"`
		Raw      map[string]any `json:"-"`
	}
	var rawClaims map[string]any
	if err = idToken.Claims(&rawClaims); err != nil {
		return err
	}
	b, _ := json.Marshal(rawClaims)
	if err = json.Unmarshal(b, &claims); err != nil {
		return err
	}
	if claims.Nonce != parts[1] {
		return errors.New("invalid OIDC nonce")
	}
	if claims.Username == "" {
		claims.Username = claims.Email
	}
	if claims.Username == "" {
		claims.Username = claims.Subject
	}
	groups := stringSliceClaim(rawClaims[cfg.GroupsClaim])
	var u store.User
	if !cfg.AutoProvision {
		u, err = m.Store.OIDCUser(r.Context(), claims.Subject)
		if errors.Is(err, pgx.ErrNoRows) {
			return errors.New("OIDC user is not provisioned in Kanvas")
		}
	} else {
		u, err = m.Store.UpsertOIDCUser(r.Context(), claims.Subject, claims.Username, claims.Name, claims.Email, cfg.AdminGroup, groups)
	}
	if err != nil {
		return fmt.Errorf("provision OIDC user: %w", err)
	}
	if err = m.NewSession(w, r, u); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{Name: "kanvas_oidc_state", Value: "", Path: "/api/v1/auth/oidc/callback", MaxAge: -1, HttpOnly: true, Secure: isSecure(r), SameSite: http.SameSiteLaxMode})
	m.Store.Audit(r.Context(), &u.ID, "LOGIN_OIDC", "USER", u.ID.String(), remoteAddr(r), r.UserAgent(), map[string]any{"issuer": cfg.Issuer})
	http.Redirect(w, r, "/", http.StatusFound)
	return nil
}

func (m *Manager) oidcClient(r *http.Request) (OIDCSettings, *oidc.Provider, *oauth2.Config, error) {
	cfg, err := m.OIDCSettings(r.Context())
	if err != nil {
		return cfg, nil, nil, err
	}
	if cfg.Issuer == "" {
		return cfg, nil, nil, errors.New("OIDC issuer is not configured")
	}
	provider, err := oidc.NewProvider(r.Context(), cfg.Issuer)
	if err != nil {
		return cfg, nil, nil, fmt.Errorf("discover OIDC issuer: %w", err)
	}
	baseURL := requestBaseURL(r)
	if raw, _, settingErr := m.Store.Setting(r.Context(), "site.base_url"); settingErr == nil {
		var configured string
		if json.Unmarshal(raw, &configured) == nil && strings.TrimSpace(configured) != "" {
			baseURL = strings.TrimRight(strings.TrimSpace(configured), "/")
		}
	}
	redirect := baseURL + "/api/v1/auth/oidc/callback"
	oauthCfg := &oauth2.Config{ClientID: cfg.ClientID, ClientSecret: cfg.ClientSecret, Endpoint: provider.Endpoint(), RedirectURL: redirect, Scopes: []string{oidc.ScopeOpenID, "profile", "email"}}
	return cfg, provider, oauthCfg, nil
}

func stringSliceClaim(v any) []string {
	raw, ok := v.([]any)
	if ok {
		out := make([]string, 0, len(raw))
		for _, x := range raw {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	if ss, ok := v.([]string); ok {
		return ss
	}
	return nil
}

func requestBaseURL(r *http.Request) string {
	scheme := "http"
	if isSecure(r) {
		scheme = "https"
	}
	host := r.Host
	if forwarded := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Host"), ",")[0]); forwarded != "" {
		host = forwarded
	}
	return scheme + "://" + host
}
func isSecure(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Proto"), ",")[0]), "https")
}
func remoteAddr(r *http.Request) string {
	if x := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-For"), ",")[0]); x != "" {
		return x
	}
	return r.RemoteAddr
}
