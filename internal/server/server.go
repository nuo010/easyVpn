package server

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"easyvpn/internal/config"
	"easyvpn/internal/control"
)

//go:embed web/index.html
var indexHTML string

type Server struct {
	cfg       config.ServerConfig
	store     *Store
	dataPlane *DataPlane
}

func New(cfg config.ServerConfig) (*Server, error) {
	allocator, err := NewIPAllocator(cfg.TunnelAddress)
	if err != nil {
		return nil, err
	}

	store := NewStore(cfg.Users, allocator, allocator.ServerAddress(), cfg.TunnelMTU)
	srv := &Server{
		cfg:   cfg,
		store: store,
	}
	if cfg.EnableTunnel {
		srv.dataPlane = NewDataPlane(cfg, store)
	}
	return srv, nil
}

func (s *Server) Run() error {
	if s.dataPlane != nil {
		if err := s.dataPlane.Start(); err != nil {
			return err
		}
	}

	adminMux := http.NewServeMux()
	adminMux.HandleFunc("/", s.handleIndex)
	adminMux.HandleFunc("/api/v1/status", s.handleStatus)
	adminMux.HandleFunc("/api/v1/login", s.handleLogin)
	adminMux.HandleFunc("/api/v1/heartbeat", s.handleHeartbeat)
	adminMux.HandleFunc("/api/v1/peers", s.handlePeers)
	adminMux.HandleFunc("/api/v1/peers/", s.handlePeerPolicy)
	adminMux.HandleFunc("/api/v1/users", s.handleUsers)
	adminMux.HandleFunc("/api/v1/users/", s.handleUserPolicy)

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
		"name":          "easyVpn",
		"listen":        s.cfg.Listen,
		"admin_bind":    s.cfg.AdminBind,
		"peer_count":    len(s.store.ListPeers()),
		"enable_tunnel": s.cfg.EnableTunnel,
		"data_addr":     s.advertisedDataAddr(nil),
		"data_tls":      s.cfg.DataTLSEnabled,
		"time":          time.Now().UTC(),
	})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req control.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if req.Username == "" || req.Password == "" {
		http.Error(w, "username and password are required", http.StatusBadRequest)
		return
	}

	user, ok := s.store.Authenticate(req.Username, req.Password)
	if !ok {
		http.Error(w, "invalid username or password", http.StatusUnauthorized)
		return
	}

	_, peer, sessionToken, err := s.store.LoginClient(req.Username, req.NodeName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, http.StatusOK, control.LoginResponse{
		PeerID:       peer.ID,
		SessionToken: sessionToken,
		Username:     user.Username,
		Policy:       peer.Policy,
		Transport: control.TransportInfo{
			DataAddr:   s.advertisedDataAddr(r),
			TLSEnabled: s.cfg.DataTLSEnabled,
		},
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

	peer, ok := s.store.TouchSession(req.SessionToken)
	if !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	writeJSON(w, http.StatusOK, control.HeartbeatResponse{
		PeerID: peer.ID,
		Policy: peer.Policy,
		Transport: control.TransportInfo{
			DataAddr:   s.advertisedDataAddr(r),
			TLSEnabled: s.cfg.DataTLSEnabled,
		},
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
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleUsers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if !checkAdminToken(r, s.cfg.AdminToken) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"users": s.store.ListUsers(),
		})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleUserPolicy(w http.ResponseWriter, r *http.Request) {
	if !checkAdminToken(r, s.cfg.AdminToken) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/v1/users/")
	if !strings.HasSuffix(path, "/policy") {
		http.NotFound(w, r)
		return
	}
	username := strings.TrimSuffix(path, "/policy")
	username = strings.TrimSuffix(username, "/")
	if username == "" {
		http.NotFound(w, r)
		return
	}

	switch r.Method {
	case http.MethodGet:
		user, ok := s.store.GetUser(username)
		if !ok {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, user.Policy)
	case http.MethodPut:
		var policy control.Policy
		if err := json.NewDecoder(r.Body).Decode(&policy); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		user, ok := s.store.UpdateUserPolicy(username, policy)
		if !ok {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, user.Policy)
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

func (s *Server) advertisedDataAddr(r *http.Request) string {
	if !s.cfg.EnableTunnel {
		return ""
	}
	if s.cfg.PublicDataAddr != "" {
		return s.cfg.PublicDataAddr
	}

	host, port, err := net.SplitHostPort(s.cfg.Listen)
	if err != nil {
		return s.cfg.Listen
	}
	if host != "" && host != "0.0.0.0" && host != "::" {
		return net.JoinHostPort(host, port)
	}
	if r == nil {
		return net.JoinHostPort("127.0.0.1", port)
	}

	requestHost := r.Host
	if parsedHost, _, err := net.SplitHostPort(r.Host); err == nil {
		requestHost = parsedHost
	}
	if requestHost == "" {
		requestHost = "127.0.0.1"
	}
	return net.JoinHostPort(requestHost, port)
}
