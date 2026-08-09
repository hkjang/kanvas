package migration

import "testing"

func TestCanonicalSpacePermission(t *testing.T) {
	tests := map[string]string{
		"VIEWSPACE":          "VIEW",
		"ADDPAGE":            "CREATE",
		"EDITSPACE":          "EDIT",
		"REMOVEPAGE":         "DELETE",
		"COMMENT":            "COMMENT",
		"CREATEATTACHMENT":   "ATTACH",
		"EXPORTSPACE":        "EXPORT",
		"SETPAGEPERMISSIONS": "RESTRICT",
		"ADMINISTERSPACE":    "SPACE_ADMIN",
	}
	for source, want := range tests {
		got, ok := canonicalSpacePermission(source)
		if !ok || got != want {
			t.Fatalf("canonicalSpacePermission(%q) = %q, %v; want %q, true", source, got, ok, want)
		}
	}
	if got, ok := canonicalSpacePermission("unknown"); ok || got != "" {
		t.Fatalf("unknown permission must fail closed, got %q, %v", got, ok)
	}
}

func TestCanonicalPagePermission(t *testing.T) {
	for _, permission := range []string{"VIEW", "EDIT", " view "} {
		got, ok := canonicalPagePermission(permission)
		if !ok || (got != "VIEW" && got != "EDIT") {
			t.Fatalf("canonicalPagePermission(%q) = %q, %v", permission, got, ok)
		}
	}
	if _, ok := canonicalPagePermission("DELETE"); ok {
		t.Fatal("unsupported page permission must fail closed")
	}
}
