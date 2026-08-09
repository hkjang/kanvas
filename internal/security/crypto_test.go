package security

import "testing"

func TestVaultRoundTrip(t *testing.T) {
	v, err := OpenVault(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	got, err := v.Encrypt("client-secret")
	if err != nil {
		t.Fatal(err)
	}
	if got == "client-secret" {
		t.Fatal("secret was not encrypted")
	}
	plain, err := v.Decrypt(got)
	if err != nil || plain != "client-secret" {
		t.Fatalf("roundtrip failed: %q %v", plain, err)
	}
}
