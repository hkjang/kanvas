package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/hkjang/kanvas/internal/auth"
	"github.com/hkjang/kanvas/internal/buildinfo"
	"github.com/hkjang/kanvas/internal/store"
)

type Handler struct{ Store *store.Store }

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}
type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}
type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		w.Header().Set("Allow", "POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id, ok := auth.IdentityFrom(r.Context())
	if !ok {
		writeHTTPError(w, http.StatusUnauthorized, "MCP authentication requires a Kanvas API key")
		return
	}
	var req request
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	if err := dec.Decode(&req); err != nil {
		writeResponse(w, response{JSONRPC: "2.0", Error: &rpcError{Code: -32700, Message: "Parse error"}})
		return
	}
	if req.JSONRPC != "2.0" {
		writeResponse(w, response{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: -32600, Message: "Invalid Request"}})
		return
	}
	var result any
	var err error
	switch req.Method {
	case "initialize":
		result = map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{"tools": map[string]any{"listChanged": false}}, "serverInfo": map[string]any{"name": "Kanvas MCP", "version": buildinfo.Version}}
	case "notifications/initialized":
		w.WriteHeader(http.StatusAccepted)
		return
	case "ping":
		result = map[string]any{}
	case "tools/list":
		result = map[string]any{"tools": toolsList()}
	case "tools/call":
		result, err = h.call(r, id, req.Params)
	default:
		writeResponse(w, response{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: -32601, Message: "Method not found"}})
		return
	}
	if err != nil {
		writeResponse(w, response{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{"content": []map[string]any{{"type": "text", "text": err.Error()}}, "isError": true}})
		return
	}
	writeResponse(w, response{JSONRPC: "2.0", ID: req.ID, Result: result})
}

func (h *Handler) call(r *http.Request, id auth.Identity, raw json.RawMessage) (any, error) {
	var p struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, err
	}
	write := p.Name == "create_page" || p.Name == "update_page" || p.Name == "add_comment"
	if write && !hasScope(id, "wiki:write") {
		return nil, errors.New("API key does not have wiki:write scope")
	}
	ctx := r.Context()
	var value any
	var err error
	switch p.Name {
	case "search_pages":
		q, _ := p.Arguments["query"].(string)
		value, err = h.Store.SearchPages(ctx, id.User.ID, q, 30)
	case "get_page":
		var pageID uuid.UUID
		pageID, err = parseUUID(p.Arguments, "pageId")
		if err == nil {
			value, err = h.Store.PageByID(ctx, id.User.ID, pageID)
		}
	case "list_spaces":
		value, err = h.Store.ListSpaces(ctx, id.User.ID)
	case "get_page_history":
		var pageID uuid.UUID
		pageID, err = parseUUID(p.Arguments, "pageId")
		if err == nil {
			value, err = h.Store.PageVersions(ctx, id.User.ID, pageID)
		}
	case "get_comments":
		var pageID uuid.UUID
		pageID, err = parseUUID(p.Arguments, "pageId")
		if err == nil {
			value, err = h.Store.Comments(ctx, id.User.ID, pageID)
		}
	case "create_page":
		var spaceID uuid.UUID
		spaceID, err = parseUUID(p.Arguments, "spaceId")
		if err != nil {
			break
		}
		title, _ := p.Arguments["title"].(string)
		content, _ := p.Arguments["content"].(string)
		if strings.TrimSpace(title) == "" {
			err = errors.New("title is required")
			break
		}
		doc := textDocument(content)
		value, err = h.Store.CreatePage(ctx, id.User.ID, spaceID, nil, title, doc, content)
	case "update_page":
		var pageID uuid.UUID
		pageID, err = parseUUID(p.Arguments, "pageId")
		if err != nil {
			break
		}
		title, _ := p.Arguments["title"].(string)
		content, _ := p.Arguments["content"].(string)
		version, ok := p.Arguments["version"].(float64)
		if !ok {
			err = errors.New("version is required")
			break
		}
		value, err = h.Store.UpdatePage(ctx, id.User.ID, pageID, title, textDocument(content), content, "MCP update", int(version))
	case "add_comment":
		var pageID uuid.UUID
		pageID, err = parseUUID(p.Arguments, "pageId")
		if err == nil {
			body, _ := p.Arguments["body"].(string)
			value, err = h.Store.AddComment(ctx, id.User.ID, pageID, nil, body)
		}
	default:
		err = fmt.Errorf("unknown tool %q", p.Name)
	}
	if err != nil {
		return nil, err
	}
	b, _ := json.MarshalIndent(value, "", "  ")
	return map[string]any{"content": []map[string]any{{"type": "text", "text": string(b)}}, "structuredContent": value}, nil
}

func toolsList() []map[string]any {
	stringProp := func(description string) map[string]any {
		return map[string]any{"type": "string", "description": description}
	}
	return []map[string]any{
		{"name": "search_pages", "description": "Search accessible Kanvas pages", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"query": stringProp("Search query")}, "required": []string{"query"}}, "annotations": map[string]any{"readOnlyHint": true}},
		{"name": "get_page", "description": "Get one accessible page", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"pageId": stringProp("Page UUID")}, "required": []string{"pageId"}}, "annotations": map[string]any{"readOnlyHint": true}},
		{"name": "list_spaces", "description": "List accessible spaces", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{}}, "annotations": map[string]any{"readOnlyHint": true}},
		{"name": "get_page_history", "description": "List immutable page versions", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"pageId": stringProp("Page UUID")}, "required": []string{"pageId"}}, "annotations": map[string]any{"readOnlyHint": true}},
		{"name": "get_comments", "description": "List page comments", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"pageId": stringProp("Page UUID")}, "required": []string{"pageId"}}, "annotations": map[string]any{"readOnlyHint": true}},
		{"name": "create_page", "description": "Create a Kanvas page", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"spaceId": stringProp("Space UUID"), "title": stringProp("Page title"), "content": stringProp("Plain text content")}, "required": []string{"spaceId", "title", "content"}}},
		{"name": "update_page", "description": "Update a page using optimistic version locking", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"pageId": stringProp("Page UUID"), "title": stringProp("Page title"), "content": stringProp("Plain text content"), "version": map[string]any{"type": "integer"}}, "required": []string{"pageId", "title", "content", "version"}}},
		{"name": "add_comment", "description": "Add a page comment", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"pageId": stringProp("Page UUID"), "body": stringProp("Comment body")}, "required": []string{"pageId", "body"}}},
	}
}

func textDocument(text string) json.RawMessage {
	nodes := []map[string]any{}
	for _, line := range strings.Split(text, "\n") {
		node := map[string]any{"type": "paragraph"}
		if line != "" {
			node["content"] = []map[string]any{{"type": "text", "text": line}}
		}
		nodes = append(nodes, node)
	}
	b, _ := json.Marshal(map[string]any{"type": "doc", "content": nodes})
	return b
}
func parseUUID(args map[string]any, key string) (uuid.UUID, error) {
	s, _ := args[key].(string)
	if s == "" {
		return uuid.Nil, fmt.Errorf("%s is required", key)
	}
	return uuid.Parse(s)
}
func hasScope(id auth.Identity, scope string) bool {
	if id.User.Role == "ADMIN" {
		return true
	}
	for _, s := range id.Scopes {
		if s == scope || s == "*" {
			return true
		}
	}
	return false
}
func writeResponse(w http.ResponseWriter, v response) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
func writeHTTPError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": message})
}
