package server

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"easyvpn/internal/control"
)

type Store struct {
	mu       sync.RWMutex
	users    map[string]control.User
	peers    map[string]control.Peer
	sessions map[string]string
}

func NewStore(users []control.User) *Store {
	userMap := make(map[string]control.User, len(users))
	for _, user := range users {
		normalized := user
		normalized.Policy = control.NormalizePolicy(user.Policy)
		userMap[normalized.Username] = normalized
	}

	return &Store{
		users:    userMap,
		peers:    make(map[string]control.Peer),
		sessions: make(map[string]string),
	}
}

func (s *Store) Authenticate(username, password string) (control.User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	user, ok := s.users[username]
	if !ok || user.Password != password {
		return control.User{}, false
	}
	return user, true
}

func (s *Store) LoginClient(username, nodeName string) (control.User, control.Peer, string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	user := s.users[username]
	now := time.Now().UTC()
	peer := control.Peer{
		ID:           randomID(),
		Name:         nodeName,
		Username:     username,
		Status:       "online",
		RegisteredAt: now,
		LastSeenAt:   now,
		Policy:       user.Policy,
	}
	s.peers[peer.ID] = peer

	sessionToken := randomID() + randomID()
	s.sessions[sessionToken] = peer.ID

	return user, peer, sessionToken
}

func (s *Store) TouchSession(sessionToken string) (control.Peer, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	peerID, ok := s.sessions[sessionToken]
	if !ok {
		return control.Peer{}, false
	}

	peer, ok := s.peers[peerID]
	if !ok {
		delete(s.sessions, sessionToken)
		return control.Peer{}, false
	}

	user, ok := s.users[peer.Username]
	if ok {
		peer.Policy = user.Policy
	}
	peer.LastSeenAt = time.Now().UTC()
	peer.Status = "online"
	s.peers[peerID] = peer
	return peer, true
}

func (s *Store) ResolveSession(sessionToken string) (control.Peer, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	peerID, ok := s.sessions[sessionToken]
	if !ok {
		return control.Peer{}, false
	}
	peer, ok := s.peers[peerID]
	return peer, ok
}

func (s *Store) ListPeers() []control.Peer {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]control.Peer, 0, len(s.peers))
	for _, peer := range s.peers {
		if time.Since(peer.LastSeenAt) > 45*time.Second {
			peer.Status = "stale"
		}
		out = append(out, peer)
	}
	return out
}

func (s *Store) GetPeer(peerID string) (control.Peer, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	peer, ok := s.peers[peerID]
	return peer, ok
}

func (s *Store) ListUsers() []control.User {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]control.User, 0, len(s.users))
	for _, user := range s.users {
		user.Password = ""
		out = append(out, user)
	}
	return out
}

func (s *Store) GetUser(username string) (control.User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	user, ok := s.users[username]
	if !ok {
		return control.User{}, false
	}
	user.Password = ""
	return user, true
}

func (s *Store) UpdateUserPolicy(username string, policy control.Policy) (control.User, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	user, ok := s.users[username]
	if !ok {
		return control.User{}, false
	}
	user.Policy = control.NormalizePolicy(policy)
	s.users[username] = user

	for id, peer := range s.peers {
		if peer.Username != username {
			continue
		}
		peer.Policy = user.Policy
		s.peers[id] = peer
	}

	user.Password = ""
	return user, true
}

func randomID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return time.Now().UTC().Format("20060102150405")
	}
	return hex.EncodeToString(buf)
}
