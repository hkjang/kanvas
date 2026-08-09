package webembed

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestHandlerServesSPAFallbackWithoutRedirect(t *testing.T) {
	root := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte(`<div id="root"></div>`)},
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/admin/users", nil)

	handlerFor(root).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if recorder.Header().Get("Location") != "" {
		t.Fatalf("unexpected redirect to %q", recorder.Header().Get("Location"))
	}
	if !strings.Contains(recorder.Body.String(), `id="root"`) {
		t.Fatalf("fallback body = %q", recorder.Body.String())
	}
}

func TestHandlerCachesFingerprintedAssets(t *testing.T) {
	root := fstest.MapFS{
		"index.html":        &fstest.MapFile{Data: []byte("app")},
		"assets/index-1.js": &fstest.MapFile{Data: []byte("script")},
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/assets/index-1.js", nil)

	handlerFor(fs.FS(root)).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if got := recorder.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Fatalf("cache control = %q", got)
	}
}
