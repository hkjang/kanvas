package mcp

import (
	"encoding/json"
	"testing"
)

func TestTextDocument(t *testing.T) {
	var v map[string]any
	if err := json.Unmarshal(textDocument("hello\nworld"), &v); err != nil {
		t.Fatal(err)
	}
	if v["type"] != "doc" {
		t.Fatalf("unexpected doc: %#v", v)
	}
}
func TestToolsIncludeACLReadAndWriteSurface(t *testing.T) {
	if len(toolsList()) != 8 {
		t.Fatalf("got %d tools", len(toolsList()))
	}
}
