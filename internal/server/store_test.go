package server

import (
	"testing"

	"easyvpn/internal/control"
)

func TestStoreLoginClientAutoAssignsAddress(t *testing.T) {
	allocator, err := NewIPAllocator("10.200.0.1/24")
	if err != nil {
		t.Fatalf("new allocator: %v", err)
	}

	store := NewStore([]control.User{
		{
			Username: "demo",
			Password: "demo123",
			Policy: control.Policy{
				Mode:   control.ModeSplit,
				Routes: []string{"10.0.0.0/8"},
			},
		},
	}, allocator, allocator.ServerAddress(), 1380)

	_, peer, _, err := store.LoginClient("demo", "laptop")
	if err != nil {
		t.Fatalf("login client: %v", err)
	}
	if peer.Policy.Tunnel.ClientAddress != "10.200.0.2/24" {
		t.Fatalf("unexpected assigned client address: %s", peer.Policy.Tunnel.ClientAddress)
	}
	if peer.Policy.Tunnel.ServerAddress != "10.200.0.1" {
		t.Fatalf("unexpected server address: %s", peer.Policy.Tunnel.ServerAddress)
	}
}

func TestStoreLoginClientReusesReleasedAddressForSameNode(t *testing.T) {
	allocator, err := NewIPAllocator("10.200.0.1/24")
	if err != nil {
		t.Fatalf("new allocator: %v", err)
	}

	store := NewStore([]control.User{
		{
			Username: "demo",
			Password: "demo123",
			Policy: control.Policy{
				Mode: control.ModeSplit,
			},
		},
	}, allocator, allocator.ServerAddress(), 1380)

	_, firstPeer, _, err := store.LoginClient("demo", "laptop")
	if err != nil {
		t.Fatalf("first login: %v", err)
	}
	_, secondPeer, _, err := store.LoginClient("demo", "laptop")
	if err != nil {
		t.Fatalf("second login: %v", err)
	}

	if firstPeer.Policy.Tunnel.ClientAddress != secondPeer.Policy.Tunnel.ClientAddress {
		t.Fatalf("expected duplicate node login to reuse released address, first=%s second=%s", firstPeer.Policy.Tunnel.ClientAddress, secondPeer.Policy.Tunnel.ClientAddress)
	}
}
