package client

import (
	"testing"

	"easyvpn/internal/control"
)

func TestBuildRoutePlanSplitMode(t *testing.T) {
	policy := control.Policy{
		Mode:   control.ModeSplit,
		Routes: []string{"10.0.0.0/8", "invalid-route", "192.168.0.0/16"},
		DNS:    []string{"1.1.1.1"},
		Tunnel: control.TunnelConfig{
			ClientAddress: "10.200.0.2/24",
			ServerAddress: "10.200.0.1",
			MTU:           1380,
		},
	}

	plan := BuildRoutePlan(policy, "203.0.113.10:8443", "easyvpn0")
	if len(plan.Routes) != 2 {
		t.Fatalf("expected two valid split routes, got %#v", plan.Routes)
	}
	if len(plan.ApplyBlockers) != 0 {
		t.Fatalf("expected no blockers for split mode, got %#v", plan.ApplyBlockers)
	}
}

func TestBuildRoutePlanFullModeRequiresEndpointIPForApply(t *testing.T) {
	policy := control.DefaultPolicy()
	policy.Mode = control.ModeFull

	plan := BuildRoutePlan(policy, "vpn.example.com:8443", "easyvpn0")
	if len(plan.ApplyBlockers) == 0 {
		t.Fatalf("expected blocker when full mode uses non-IP endpoint")
	}
}

func TestBuildRoutePlanFullModeAddsEndpointBypass(t *testing.T) {
	policy := control.DefaultPolicy()
	policy.Mode = control.ModeFull

	plan := BuildRoutePlan(policy, "203.0.113.10:8443", "easyvpn0")
	if len(plan.Routes) != 3 {
		t.Fatalf("expected one bypass route and two default split routes, got %#v", plan.Routes)
	}
	if plan.Routes[0].Kind != routeKindBypass {
		t.Fatalf("expected first route to be endpoint bypass, got %#v", plan.Routes[0])
	}
}
