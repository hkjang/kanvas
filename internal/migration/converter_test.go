package migration

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestConvertStoragePreservesStructureAndMacros(t *testing.T) {
	source := `<h1>Runbook</h1><p>Hello <strong>team</strong>.</p><ac:structured-macro ac:name="code"><ac:parameter ac:name="language">go</ac:parameter><ac:plain-text-body>fmt.Println("ok")</ac:plain-text-body></ac:structured-macro><ac:structured-macro ac:name="custom-risk"><ac:rich-text-body><p>Keep me</p></ac:rich-text-body></ac:structured-macro><ri:page ri:content-title="Operations"/>`
	got := ConvertStorage(source)
	if !strings.Contains(got.Text, "Runbook") || !strings.Contains(got.Text, "Keep me") {
		t.Fatalf("text lost: %q", got.Text)
	}
	if len(got.Macros) != 2 || !got.Macros[0].Supported || got.Macros[1].Supported {
		t.Fatalf("unexpected macros: %#v", got.Macros)
	}
	if len(got.Links) != 1 || got.Links[0].Target != "Operations" {
		t.Fatalf("unexpected links: %#v", got.Links)
	}
	var editor map[string]any
	if err := json.Unmarshal(got.Editor, &editor); err != nil {
		t.Fatal(err)
	}
	if editor["type"] != "doc" {
		t.Fatalf("not an editor doc: %s", got.Editor)
	}
}

func TestConvertStorageCountsRepeatedMacros(t *testing.T) {
	got := ConvertStorage(`<ac:structured-macro ac:name="info"/><ac:structured-macro ac:name="info"/>`)
	if len(got.Macros) != 1 || got.Macros[0].Occurrences != 2 {
		t.Fatalf("unexpected macro count: %#v", got.Macros)
	}
}

func TestConvertMalformedStorageFallsBackWithoutDroppingText(t *testing.T) {
	got := ConvertStorage(`<p>Broken <strong>content</p>`)
	if !strings.Contains(got.Text, "Broken") || len(got.Warnings) == 0 {
		t.Fatalf("fallback failed: %#v", got)
	}
}
