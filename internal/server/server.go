package server

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"easyvpn/internal/config"
	"easyvpn/internal/control"
)

//go:embed web/index.html
var indexHTML string

type Server struct {
	cfg   config.ServerConfig
	store *Store
}

func New(cfg config.ServerConfig) *Server {
	return &Server{
		cfg:   cfg,
		store: NewStore(cfg.EnrollmentTokens),
	}
}

func (s *Server) Run() error {
	adminMux := http.NewServeMux()
	adminMux.HandleFunc("/", s.handleIndex)
	adminMux.HandleFunc("/api/v1/status", s.handleStatus)
	adminMux.HandleFunc("/api/v1/register", s.handleRegister)
	adminMux.HandleFunc("/api/v1/heartbeat", s.handleHeartbeat)
	adminMux.HandleFunc("/api/v1/peers", s.handlePeers)
	adminMux.HandleFunc("/api/v1/peers/", s.handlePeerPolicy)

	adminServer := &http.Server{
		Addr:              s.cfg.AdminBind,
		Handler:           s.loggingMiddleware(adminMux),
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("easyVpn admin server listening on %s", s.cfg.AdminBind)
	err := adminServer.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return adminServer.Shutdown(context.Background())
}

func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(indexHTML))
}

func (s *Server) handleStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"name":       "easyVpn",
		"listen":     s.cfg.Listen,
		"admin_bind": s.cfg.AdminBind,
		"peer_count": len(s.store.ListPeers()),
		"time":       time.Now().UTC(),
	})
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req control.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if !s.store.IsEnrollmentTokenValid(req.Token) {
		http.Error(w, "invalid enrollment token", http.StatusUnauthorized)
		return
	}

	peer := s.store.RegisterPeer(req.NodeName)
	writeJSON(w, http.StatusOK, control.RegisterResponse{
		PeerID: peer.ID,
		Policy: peer.Policy,
	})
}

func (s *Server) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req control.HeartbeatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	peer, ok := s.store.TouchPeer(req.PeerID)
	if !ok {
		http.Error(w, "peer not found", http.StatusNotFound)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"peer_id": req.PeerID,
		"policy":  peer.Policy,
	})
}

func (s *Server) handlePeers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{
			"peers": s.store.ListPeers(),
		})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handlePeerPolicy(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/peers/")
	if !strings.HasSuffix(path, "/policy") {
		http.NotFound(w, r)
		return
	}
	peerID := strings.TrimSuffix(path, "/policy")
	peerID = strings.TrimSuffix(peerID, "/")
	if peerID == "" {
		http.NotFound(w, r)
		return
	}

	switch r.Method {
	case http.MethodGet:
		peer, ok := s.store.GetPeer(peerID)
		if !ok {
			http.Error(w, "peer not found", http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, peer.Policy)
	case http.MethodPut:
		if !checkAdminToken(r, s.cfg.AdminToken) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var policy control.Policy
		if err := json.NewDecoder(r.Body).Decode(&policy); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		peer, ok := s.store.UpdatePolicy(peerID, policy)
		if !ok {
			http.Error(w, "peer not found", http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, peer.Policy)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func checkAdminToken(r *http.Request, expected string) bool {
	if expected == "" {
		return true
	}
	return r.Header.Get("X-Admin-Token") == expected
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
