package client

import (
	"fmt"
	"log"
	"net"
	"net/netip"
	"net/url"
	"os/exec"
	"runtime"
	"slices"
	"strings"

	"easyvpn/internal/control"
)

type routeKind string

const (
	routeKindTunnel routeKind = "tunnel"
	routeKindBypass routeKind = "bypass"
)

type ManagedRoute struct {
	Destination string    `json:"destination"`
	Device      string    `json:"device"`
	Kind        routeKind `json:"kind"`
}

type RoutePlan struct {
	Mode          control.Mode   `json:"mode"`
	TunnelName    string         `json:"tunnel_name"`
	DNS           []string       `json:"dns"`
	Routes        []ManagedRoute `json:"routes"`
	Warnings      []string       `json:"warnings"`
	ApplyBlockers []string       `json:"apply_blockers"`
}

type RouteApplier struct {
	apply bool
}

func NewRouteApplier(apply bool) *RouteApplier {
	return &RouteApplier{apply: apply}
}

func BuildRoutePlan(policy control.Policy, dataAddr, tunnelName string) RoutePlan {
	if tunnelName == "" {
		tunnelName = "easyvpn0"
	}

	plan := RoutePlan{
		Mode:       policy.Mode,
		TunnelName: tunnelName,
		DNS:        slices.Clone(policy.DNS),
	}

	switch policy.Mode {
	case control.ModeFull:
		plan.Routes = append(plan.Routes,
			ManagedRoute{Destination: "0.0.0.0/1", Device: tunnelName, Kind: routeKindTunnel},
			ManagedRoute{Destination: "128.0.0.0/1", Device: tunnelName, Kind: routeKindTunnel},
		)
	default:
		for _, raw := range policy.Routes {
			if _, err := netip.ParsePrefix(raw); err != nil {
				plan.Warnings = append(plan.Warnings, "skip invalid route "+raw)
				continue
			}
			plan.Routes = append(plan.Routes, ManagedRoute{
				Destination: raw,
				Device:      tunnelName,
				Kind:        routeKindTunnel,
			})
		}
	}

	endpointHost, endpointAddr, ok := parseServerEndpoint(dataAddr)
	if policy.Mode == control.ModeFull {
		if !ok {
			plan.ApplyBlockers = append(plan.ApplyBlockers, "full tunnel route apply currently requires data_addr to use a literal IP address")
		} else {
			plan.Routes = append([]ManagedRoute{{
				Destination: prefixForAddr(endpointAddr),
				Kind:        routeKindBypass,
			}}, plan.Routes...)
			plan.Warnings = append(plan.Warnings, "server endpoint "+endpointHost+" will be kept outside the tunnel")
		}
	}

	if policy.Tunnel.ClientAddress == "" {
		plan.Warnings = append(plan.Warnings, "policy tunnel.client_address is empty; TUN setup is not ready yet")
	}
	if policy.Tunnel.ServerAddress == "" {
		plan.Warnings = append(plan.Warnings, "policy tunnel.server_address is empty; gateway routing is incomplete")
	}

	return plan
}

func RoutePlansEqual(a, b RoutePlan) bool {
	return a.Mode == b.Mode &&
		a.TunnelName == b.TunnelName &&
		slices.Equal(a.DNS, b.DNS) &&
		slices.Equal(a.Warnings, b.Warnings) &&
		slices.Equal(a.ApplyBlockers, b.ApplyBlockers) &&
		slices.EqualFunc(a.Routes, b.Routes, func(left, right ManagedRoute) bool {
			return left == right
		})
}

func (p RoutePlan) Summary() string {
	parts := []string{
		"mode=" + string(p.Mode),
		"device=" + p.TunnelName,
		fmt.Sprintf("routes=%d", len(p.Routes)),
	}
	if len(p.DNS) > 0 {
		parts = append(parts, "dns="+strings.Join(p.DNS, ","))
	}
	if len(p.ApplyBlockers) > 0 {
		parts = append(parts, "blockers="+strings.Join(p.ApplyBlockers, "; "))
	}
	return strings.Join(parts, " ")
}

func (a *RouteApplier) Sync(previous, next RoutePlan) error {
	toAdd, toDelete := diffRoutes(previous, next)

	log.Printf("route sync plan: add=%d remove=%d %s", len(toAdd), len(toDelete), next.Summary())
	for _, warning := range next.Warnings {
		log.Printf("route warning: %s", warning)
	}

	if !a.apply {
		for _, route := range toAdd {
			log.Printf("route dry-run add: %s %s via %s", route.Kind, route.Destination, route.Device)
		}
		for _, route := range toDelete {
			log.Printf("route dry-run remove: %s %s", route.Kind, route.Destination)
		}
		return nil
	}

	if runtime.GOOS != "linux" {
		return fmt.Errorf("apply_system_routes is currently supported only on Linux")
	}
	if len(next.ApplyBlockers) > 0 {
		return fmt.Errorf("cannot apply route plan: %s", strings.Join(next.ApplyBlockers, "; "))
	}

	slices.SortStableFunc(toAdd, func(left, right ManagedRoute) int {
		if left.Kind == right.Kind {
			return strings.Compare(left.Destination, right.Destination)
		}
		if left.Kind == routeKindBypass {
			return -1
		}
		return 1
	})

	for _, route := range toAdd {
		if err := a.addRoute(route); err != nil {
			return err
		}
	}

	slices.SortStableFunc(toDelete, func(left, right ManagedRoute) int {
		if left.Kind == right.Kind {
			return strings.Compare(left.Destination, right.Destination)
		}
		if left.Kind == routeKindTunnel {
			return -1
		}
		return 1
	})

	for _, route := range toDelete {
		if err := a.deleteRoute(route); err != nil {
			return err
		}
	}

	return nil
}

func diffRoutes(previous, next RoutePlan) (toAdd []ManagedRoute, toDelete []ManagedRoute) {
	previousMap := make(map[string]ManagedRoute, len(previous.Routes))
	nextMap := make(map[string]ManagedRoute, len(next.Routes))

	for _, route := range previous.Routes {
		previousMap[routeKey(route)] = route
	}
	for _, route := range next.Routes {
		nextMap[routeKey(route)] = route
	}

	for key, route := range nextMap {
		if _, ok := previousMap[key]; !ok {
			toAdd = append(toAdd, route)
		}
	}
	for key, route := range previousMap {
		if _, ok := nextMap[key]; !ok {
			toDelete = append(toDelete, route)
		}
	}

	return toAdd, toDelete
}

func routeKey(route ManagedRoute) string {
	return string(route.Kind) + "|" + route.Destination + "|" + route.Device
}

func parseServerEndpoint(endpoint string) (string, netip.Addr, bool) {
	if endpoint == "" {
		return "", netip.Addr{}, false
	}

	host := endpoint
	if strings.Contains(endpoint, "://") {
		parsed, err := url.Parse(endpoint)
		if err != nil {
			return "", netip.Addr{}, false
		}
		host = parsed.Hostname()
	} else if parsedHost, _, err := net.SplitHostPort(endpoint); err == nil {
		host = parsedHost
	}

	addr, err := netip.ParseAddr(host)
	if err != nil {
		return host, netip.Addr{}, false
	}
	return host, addr, true
}

func prefixForAddr(addr netip.Addr) string {
	if addr.Is4() {
		return addr.String() + "/32"
	}
	return addr.String() + "/128"
}

func (a *RouteApplier) addRoute(route ManagedRoute) error {
	switch route.Kind {
	case routeKindBypass:
		return a.addBypassRoute(route)
	case routeKindTunnel:
		return runIP("route", "replace", route.Destination, "dev", route.Device)
	default:
		return fmt.Errorf("unsupported route kind %q", route.Kind)
	}
}

func (a *RouteApplier) deleteRoute(route ManagedRoute) error {
	return runIP("route", "del", route.Destination)
}

func (a *RouteApplier) addBypassRoute(route ManagedRoute) error {
	target := route.Destination
	if idx := strings.IndexByte(target, '/'); idx >= 0 {
		target = target[:idx]
	}

	resolved, err := lookupLinuxRoute(target)
	if err != nil {
		return fmt.Errorf("resolve route for server endpoint %s: %w", target, err)
	}
	if resolved.Dev == "" {
		return fmt.Errorf("route lookup for %s did not return a device", target)
	}
	if resolved.Via == "" {
		return runIP("route", "replace", route.Destination, "dev", resolved.Dev, "scope", "link")
	}
	return runIP("route", "replace", route.Destination, "via", resolved.Via, "dev", resolved.Dev)
}

type resolvedLinuxRoute struct {
	Via string
	Dev string
}

func lookupLinuxRoute(target string) (resolvedLinuxRoute, error) {
	output, err := exec.Command("ip", "route", "get", target).CombinedOutput()
	if err != nil {
		return resolvedLinuxRoute{}, fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}

	fields := strings.Fields(string(output))
	resolved := resolvedLinuxRoute{}
	for idx := 0; idx < len(fields)-1; idx++ {
		switch fields[idx] {
		case "via":
			resolved.Via = fields[idx+1]
		case "dev":
			resolved.Dev = fields[idx+1]
		}
	}
	return resolved, nil
}

func runIP(args ...string) error {
	output, err := exec.Command("ip", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("ip %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}
