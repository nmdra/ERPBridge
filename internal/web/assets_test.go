package web

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func TestAssetsServeFallbackAndSPAPaths(t *testing.T) {
	assets := fstest.MapFS{
		"index-fallback.html": &fstest.MapFile{Data: []byte("fallback sentinel")},
	}
	handler := newAssetHandler(assets)

	for _, path := range []string{"/", "/overview", "/topology"} {
		t.Run(path, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1"+path, nil)
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
			}
			if recorder.Body.String() != "fallback sentinel" {
				t.Fatalf("body = %q", recorder.Body.String())
			}
		})
	}
}

func TestAssetsServeGeneratedAssetsAndCachesHashes(t *testing.T) {
	assets := fstest.MapFS{
		"index.html":           &fstest.MapFile{Data: []byte("generated index")},
		"assets/app-abc123.js": &fstest.MapFile{Data: []byte("console.log('ok')")},
		"index-fallback.html":  &fstest.MapFile{Data: []byte("fallback sentinel")},
	}
	handler := newAssetHandler(assets)

	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/assets/app-abc123.js", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || recorder.Body.String() != "console.log('ok')" {
		t.Fatalf("asset response = %d %q", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Fatalf("cache header = %q", got)
	}

	request = httptest.NewRequest(http.MethodGet, "http://127.0.0.1/overview", nil)
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || recorder.Body.String() != "generated index" {
		t.Fatalf("SPA response = %d %q", recorder.Code, recorder.Body.String())
	}
}

func TestAssetsRejectTraversal(t *testing.T) {
	assets := fstest.MapFS{
		"index-fallback.html": &fstest.MapFile{Data: []byte("fallback sentinel")},
		"secret.txt":          &fstest.MapFile{Data: []byte("secret")},
	}
	handler := newAssetHandler(assets)
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/../secret.txt", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || recorder.Body.String() == "secret" {
		t.Fatalf("traversal response = %d %q", recorder.Code, recorder.Body.String())
	}

	if _, err := fs.Stat(assets, "secret.txt"); err != nil {
		t.Fatal(err)
	}
}
