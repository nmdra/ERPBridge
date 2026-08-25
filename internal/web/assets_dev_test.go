package web

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func TestAssetsProxyOnlyExplicitDevTraffic(t *testing.T) {
	devServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/src/main.tsx" {
			t.Fatalf("dev path = %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "dev source")
	}))
	defer devServer.Close()

	assets := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("generated index")},
	}
	handler := newAssetHandler(assets, devServer.URL)
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/src/main.tsx", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || recorder.Body.String() != "dev source" {
		t.Fatalf("dev response = %d %q", recorder.Code, recorder.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "http://127.0.0.1/assets/app.js", nil)
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Body.String() == "dev source" {
		t.Fatal("generated asset traffic was proxied to Vite")
	}
}
