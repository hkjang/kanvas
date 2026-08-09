package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestConnectionFingerprintDoesNotLeakDSN(t *testing.T) {
	dsn := "postgres://user:supersecret@db/kanvas"
	got := connectionFingerprint(dsn)
	if got["configured"] != true {
		t.Fatal(got)
	}
	if got["fingerprint"] == dsn {
		t.Fatal("DSN leaked")
	}
}
func TestValidScopes(t *testing.T) {
	if !validScopes([]string{"wiki:read", "wiki:write"}) || validScopes([]string{"admin"}) || validScopes(nil) {
		t.Fatal("scope validation failed")
	}
}

func TestNotImplementedMigrationStepIsExplicit(t *testing.T) {
	recorder := httptest.NewRecorder()
	respond(recorder, nil, errors.New("CDC engine is not implemented in this release; transition is fail-closed"))
	if recorder.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotImplemented)
	}
}

func TestValidateManagedSetting(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		value   any
		wantErr bool
	}{
		{name: "site name", key: "site.name", value: "Engineering Wiki"},
		{name: "empty site name", key: "site.name", value: " ", wantErr: true},
		{name: "base URL", key: "site.base_url", value: "https://wiki.example.internal"},
		{name: "base URL can be cleared", key: "site.base_url", value: ""},
		{name: "base URL rejects credentials", key: "site.base_url", value: "https://admin:secret@wiki.example.internal", wantErr: true},
		{name: "base URL rejects relative URL", key: "site.base_url", value: "/wiki", wantErr: true},
		{name: "session minimum", key: "security.session_hours", value: float64(1)},
		{name: "session maximum", key: "security.session_hours", value: float64(168)},
		{name: "session out of range", key: "security.session_hours", value: float64(169), wantErr: true},
		{name: "session rejects fraction", key: "security.session_hours", value: 1.5, wantErr: true},
		{name: "batch size", key: "migration.batch_size", value: float64(500)},
		{name: "batch too small", key: "migration.batch_size", value: float64(9), wantErr: true},
		{name: "workers", key: "migration.parallelism", value: float64(8)},
		{name: "workers too many", key: "attachments.copy_threads", value: float64(33), wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateManagedSetting(test.key, test.value)
			if (err != nil) != test.wantErr {
				t.Fatalf("validateManagedSetting(%q, %#v) error = %v, wantErr %v", test.key, test.value, err, test.wantErr)
			}
		})
	}
}
