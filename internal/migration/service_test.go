package migration

import "testing"

func TestKnownTables(t *testing.T) {
	m := knownTables()
	if !m["CONTENT"] || m["AO_123"] {
		t.Fatal("unexpected table classification")
	}
}
func TestTransitionGraphRejectsShortcut(t *testing.T) {
	if transitions["LEGACY"]["CUTOVER"] {
		t.Fatal("unsafe shortcut allowed")
	}
}

func TestExceptionCheckStatus(t *testing.T) {
	tests := []struct {
		name       string
		open       int64
		approved   int64
		openStatus string
		want       string
	}{
		{name: "clean", openStatus: "WARNING", want: "PASS"},
		{name: "approved risk", approved: 3, openStatus: "WARNING", want: "APPROVED"},
		{name: "macro remains open", open: 1, approved: 3, openStatus: "WARNING", want: "WARNING"},
		{name: "orphan remains open", open: 1, openStatus: "FAIL", want: "FAIL"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := exceptionCheckStatus(test.open, test.approved, test.openStatus); got != test.want {
				t.Fatalf("exceptionCheckStatus() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestValidUnsupportedStatus(t *testing.T) {
	for _, status := range []string{"OPEN", "APPROVED", "RESOLVED"} {
		if !validUnsupportedStatus(status) {
			t.Fatalf("%s should be valid", status)
		}
	}
	for _, status := range []string{"", "open", "IGNORED"} {
		if validUnsupportedStatus(status) {
			t.Fatalf("%q should be invalid", status)
		}
	}
}
