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

type Policy struct {
	Mode        Mode      `json:"mode"`
	Routes      []string  `json:"routes"`
	DNS         []string  `json:"dns"`
	DefaultPeer string    `json:"default_peer"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type RegisterRequest struct {
	Token    string `json:"token"`
	NodeName string `json:"node_name"`
}

type RegisterResponse struct {
	PeerID string `json:"peer_id"`
	Policy Policy `json:"policy"`
}

type HeartbeatRequest struct {
	PeerID      string    `json:"peer_id"`
	Version     string    `json:"version"`
	ReportedAt  time.Time `json:"reported_at"`
	LocalIPHint string    `json:"local_ip_hint"`
}

type Peer struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
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

func DefaultPolicy() Policy {
	return Policy{
		Mode:      ModeSplit,
		Routes:    []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"},
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
	policy.UpdatedAt = time.Now().UTC()
	return policy
}
