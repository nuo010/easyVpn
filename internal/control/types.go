package control

import (
	"net/netip"
	"slices"
	"time"
)

type Mode string

const (
	ModeSplit Mode = "split"
	ModeFull  Mode = "full"
)

type TunnelConfig struct {
	ClientAddress string `json:"client_address"`
	ServerAddress string `json:"server_address"`
	MTU           int    `json:"mtu"`
}

type TransportInfo struct {
	DataAddr   string `json:"data_addr"`
	TLSEnabled bool   `json:"tls_enabled"`
}

type Policy struct {
	Mode        Mode         `json:"mode"`
	Routes      []string     `json:"routes"`
	DNS         []string     `json:"dns"`
	DefaultPeer string       `json:"default_peer"`
	Tunnel      TunnelConfig `json:"tunnel"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

type User struct {
	Username string `json:"username"`
	Password string `json:"password,omitempty"`
	Policy   Policy `json:"policy"`
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	NodeName string `json:"node_name"`
}

type LoginResponse struct {
	PeerID       string        `json:"peer_id"`
	SessionToken string        `json:"session_token"`
	Username     string        `json:"username"`
	Policy       Policy        `json:"policy"`
	Transport    TransportInfo `json:"transport"`
}

type HeartbeatRequest struct {
	SessionToken string    `json:"session_token"`
	PeerID       string    `json:"peer_id"`
	Version      string    `json:"version"`
	ReportedAt   time.Time `json:"reported_at"`
	LocalIPHint  string    `json:"local_ip_hint"`
}

type HeartbeatResponse struct {
	PeerID    string        `json:"peer_id"`
	Policy    Policy        `json:"policy"`
	Transport TransportInfo `json:"transport"`
}

type Peer struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Username     string    `json:"username"`
	Status       string    `json:"status"`
	RegisteredAt time.Time `json:"registered_at"`
	LastSeenAt   time.Time `json:"last_seen_at"`
	Policy       Policy    `json:"policy"`
}

type RouteDecision struct {
	Destination string `json:"destination"`
	UseTunnel   bool   `json:"use_tunnel"`
	Reason      string `json:"reason"`
}

type DataConnectRequest struct {
	SessionToken string `json:"session_token"`
	PeerID       string `json:"peer_id"`
}

type DataConnectResponse struct {
	PeerID  string `json:"peer_id"`
	Message string `json:"message"`
}

func DefaultPolicy() Policy {
	return Policy{
		Mode:   ModeSplit,
		Routes: []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"},
		DNS:    []string{"1.1.1.1", "8.8.8.8"},
		Tunnel: TunnelConfig{
			MTU: 1380,
		},
		UpdatedAt: time.Now().UTC(),
	}
}

func EvaluateRoute(policy Policy, destination string) RouteDecision {
	if policy.Mode == ModeFull {
		return RouteDecision{
			Destination: destination,
			UseTunnel:   true,
			Reason:      "full tunnel mode",
		}
	}

	addr, err := netip.ParseAddr(destination)
	if err != nil {
		return RouteDecision{
			Destination: destination,
			UseTunnel:   false,
			Reason:      "invalid destination IP",
		}
	}

	for _, raw := range policy.Routes {
		prefix, err := netip.ParsePrefix(raw)
		if err != nil {
			continue
		}
		if prefix.Contains(addr) {
			return RouteDecision{
				Destination: destination,
				UseTunnel:   true,
				Reason:      "matched split-tunnel route " + raw,
			}
		}
	}

	return RouteDecision{
		Destination: destination,
		UseTunnel:   false,
		Reason:      "destination not in split-tunnel routes",
	}
}

func NormalizePolicy(policy Policy) Policy {
	if policy.Mode != ModeFull {
		policy.Mode = ModeSplit
	}
	policy.Routes = slices.Clone(policy.Routes)
	policy.DNS = slices.Clone(policy.DNS)
	if policy.Tunnel.MTU <= 0 {
		policy.Tunnel.MTU = 1380
	}
	policy.UpdatedAt = time.Now().UTC()
	return policy
}

func PoliciesEqual(a, b Policy) bool {
	return a.Mode == b.Mode &&
		a.DefaultPeer == b.DefaultPeer &&
		a.Tunnel == b.Tunnel &&
		slices.Equal(a.Routes, b.Routes) &&
		slices.Equal(a.DNS, b.DNS)
}
