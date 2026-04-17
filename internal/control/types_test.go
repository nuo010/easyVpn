package control

import "testing"

func TestEvaluateRouteSplitMode(t *testing.T) {
	policy := Policy{
		Mode:   ModeSplit,
		Routes: []string{"10.0.0.0/8", "192.168.0.0/16"},
	}

	got := EvaluateRoute(policy, "10.2.3.4")
	if !got.UseTunnel {
		t.Fatalf("expected split route to use tunnel, got %#v", got)
	}

	got = EvaluateRoute(policy, "8.8.8.8")
	if got.UseTunnel {
		t.Fatalf("expected public IP to bypass tunnel, got %#v", got)
	}
}

func TestEvaluateRouteFullMode(t *testing.T) {
	policy := Policy{Mode: ModeFull}
	got := EvaluateRoute(policy, "8.8.8.8")
	if !got.UseTunnel {
		t.Fatalf("expected full tunnel mode to force tunnel, got %#v", got)
	}
}
