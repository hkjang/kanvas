package api

import "testing"

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
