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
