package mcp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nmdra/ERPBridge/internal/cache"
	"github.com/nmdra/ERPBridge/internal/logger"
)

func TestServerInfoReturnsSafeBuildAndRuntimeMetadata(t *testing.T) {
	server := NewServer(nil, cache.NewMemoryManager(4, logger.Init()), logger.Init(), RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")
	server.SetServerInfo(ServerInfo{Version: "v1.2.3", Commit: "abc123", Date: "2026-08-25"})
	mux := http.NewServeMux()
	server.ServeHTTP(mux, "http://localhost:8080")

	request := httptest.NewRequest(http.MethodGet, "http://localhost:8080/api/info", nil)
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	var info ServerInfo
	if err := json.Unmarshal(recorder.Body.Bytes(), &info); err != nil {
		t.Fatal(err)
	}
	if info.Version != "v1.2.3" || info.Commit != "abc123" || info.CacheBackend != "memory" || info.ObservedAt.IsZero() {
		t.Fatalf("server info = %+v", info)
	}
}
