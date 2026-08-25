package mcp

import (
	"encoding/json"
	"net/http"
	"time"
)

// ServerInfo contains safe build and runtime metadata.
type ServerInfo struct {
	Version         string    `json:"version"`
	Commit          string    `json:"commit,omitempty"`
	Date            string    `json:"date,omitempty"`
	CacheBackend    string    `json:"cacheBackend"`
	ActiveToolCount int       `json:"activeToolCount"`
	ObservedAt      time.Time `json:"observedAt"`
}

// SetServerInfo supplies safe build identity to the server.
func (s *Server) SetServerInfo(info ServerInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.serverInfo = info
}

// CurrentServerInfo returns a snapshot with safe runtime values.
func (s *Server) CurrentServerInfo() ServerInfo {
	s.mu.RLock()
	info := s.serverInfo
	s.mu.RUnlock()
	if info.CacheBackend == "" {
		info.CacheBackend = "disabled"
		if s.cache != nil {
			info.CacheBackend = s.cache.BackendName()
		}
	}
	if s.store != nil {
		if tools, err := s.store.List(); err == nil {
			for _, tool := range tools {
				if tool.Metadata.IsActive {
					info.ActiveToolCount++
				}
			}
		}
	}
	info.ObservedAt = time.Now().UTC()
	return info
}

func (s *Server) handleInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.CurrentServerInfo())
}
