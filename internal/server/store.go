package server

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"easyvpn/internal/control"
)

type Store struct {
	mu               sync.RWMutex
	enrollmentTokens map[string]struct{}
	peers            map[string]control.Peer
}

func NewStore(tokens []string) *Store {
	tokenSet := make(map[string]struct{}, len(tokens))
	for _, token := range tokens {
		tokenSet[token] = struct{}{}
	}

	return &Store{
		enrollmentTokens: tokenSet,
		peers:            make(map[string]control.Peer),
	}
}

func (s *Store) IsEnrollmentTokenValid(token string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.enrollmentTokens[token]
	return ok
}

func (s *Store) RegisterPeer(name string) control.Peer {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	peer := control.Peer{
		ID:           randomID(),
		Name:         name,
		Status:       "online",
		RegisteredAt: now,
		LastSeenAt:   now,
		Policy:       control.DefaultPolicy(),
	}
	s.peers[peer.ID] = peer
	return peer
}

func (s *Store) TouchPeer(peerID string) (control.Peer, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	peer, ok := s.peers[peerID]
	if !ok {
		return control.Peer{}, false
	}
	peer.LastSeenAt = time.Now().UTC()
	peer.Status = "online"
	s.peers[peerID] = peer
	return peer, true
}

func (s *Store) GetPeer(peerID string) (control.Peer, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
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

func (s *Store) UpdatePolicy(peerID string, policy control.Policy) (control.Peer, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	peer, ok := s.peers[peerID]
	if !ok {
		return control.Peer{}, false
	}
	peer.Policy = control.NormalizePolicy(policy)
	s.peers[peerID] = peer
	return peer, true
}

func randomID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return time.Now().UTC().Format("20060102150405")
	}
	return hex.EncodeToString(buf)
}
