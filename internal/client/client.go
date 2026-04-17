package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"easyvpn/internal/config"
	"easyvpn/internal/control"
)

const version = "0.1.0"

type Client struct {
	cfg          config.ClientConfig
	httpClient   *http.Client
	routeApplier *RouteApplier
	routePlan    RoutePlan
	dataPlane    *DataPlane
	peerID       string
	sessionToken string
	username     string
	transport    control.TransportInfo
	policy       control.Policy
}

func New(cfg config.ClientConfig) *Client {
	return &Client{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		routeApplier: NewRouteApplier(cfg.ApplySystemRoutes),
	}
}

func (c *Client) Run() error {
	if err := c.login(); err != nil {
		return err
	}
	log.Printf("logged in as %s, peer %s", c.username, c.peerID)
	if err := c.syncPolicy("initial login"); err != nil {
		return err
	}

	heartbeatTicker := time.NewTicker(c.cfg.HeartbeatInterval)
	defer heartbeatTicker.Stop()

	decisionTicker := time.NewTicker(c.cfg.PollInterval)
	defer decisionTicker.Stop()

	for {
		select {
		case <-heartbeatTicker.C:
			if err := c.heartbeat(); err != nil {
				log.Printf("heartbeat failed: %v", err)
			}
		case <-decisionTicker.C:
			c.printDecisionExamples()
		}
	}
}

func (c *Client) login() error {
	resp := control.LoginResponse{}
	if err := c.post("/api/v1/login", control.LoginRequest{
		Username: c.cfg.Username,
		Password: c.cfg.Password,
		NodeName: c.cfg.NodeName,
	}, &resp); err != nil {
		return fmt.Errorf("login: %w", err)
	}
	c.peerID = resp.PeerID
	c.sessionToken = resp.SessionToken
	c.username = resp.Username
	c.transport = resp.Transport
	c.policy = control.NormalizePolicy(resp.Policy)
	return nil
}

func (c *Client) heartbeat() error {
	var resp control.HeartbeatResponse

	if err := c.post("/api/v1/heartbeat", control.HeartbeatRequest{
		SessionToken: c.sessionToken,
		PeerID:       c.peerID,
		Version:      version,
		ReportedAt:   time.Now().UTC(),
		LocalIPHint:  firstNonLoopbackIPv4(),
	}, &resp); err != nil {
		return err
	}

	nextPolicy := control.NormalizePolicy(resp.Policy)
	nextTransport := resp.Transport
	if !control.PoliciesEqual(c.policy, nextPolicy) || c.transport != nextTransport {
		c.policy = nextPolicy
		c.transport = nextTransport
		if err := c.syncPolicy("heartbeat update"); err != nil {
			return err
		}
	}
	log.Printf("heartbeat ok; mode=%s routes=%v", c.policy.Mode, c.policy.Routes)
	return nil
}

func (c *Client) post(path string, payload any, target any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(c.cfg.ServerURL, "/")+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(res.Body, 1024))
		return fmt.Errorf("http %d: %s", res.StatusCode, strings.TrimSpace(string(msg)))
	}

	if target == nil {
		return nil
	}
	return json.NewDecoder(res.Body).Decode(target)
}

func (c *Client) printPolicy() {
	log.Printf(
		"policy mode=%s routes=%v dns=%v tunnel.client=%s tunnel.server=%s mtu=%d",
		c.policy.Mode,
		c.policy.Routes,
		c.policy.DNS,
		c.policy.Tunnel.ClientAddress,
		c.policy.Tunnel.ServerAddress,
		c.policy.Tunnel.MTU,
	)
}

func (c *Client) printDecisionExamples() {
	for _, destination := range []string{"1.1.1.1", "8.8.8.8", "10.10.1.20", "192.168.50.3"} {
		decision := control.EvaluateRoute(c.policy, destination)
		log.Printf("route decision: dst=%s use_tunnel=%v reason=%s", decision.Destination, decision.UseTunnel, decision.Reason)
	}
}

func firstNonLoopbackIPv4() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipNet.IP.To4()
			if ip == nil || ip.IsLoopback() {
				continue
			}
			return ip.String()
		}
	}
	return ""
}

func (c *Client) syncPolicy(source string) error {
	nextPlan := BuildRoutePlan(c.policy, c.transport.DataAddr, c.cfg.TunnelName)
	if RoutePlansEqual(c.routePlan, nextPlan) {
		return c.syncDataPlane()
	}
	if err := c.routeApplier.Sync(c.routePlan, nextPlan); err != nil {
		return fmt.Errorf("sync routes after %s: %w", source, err)
	}
	c.routePlan = nextPlan
	c.printPolicy()
	log.Printf("route plan synced after %s: %s", source, nextPlan.Summary())
	return c.syncDataPlane()
}

func (c *Client) syncDataPlane() error {
	if !c.cfg.EnableTunnel {
		return nil
	}

	if c.dataPlane == nil {
		dataPlane := NewDataPlane(c.cfg, c.peerID, c.sessionToken)
		if err := dataPlane.Start(c.transport, c.policy); err != nil {
			return err
		}
		c.dataPlane = dataPlane
		return nil
	}
	return c.dataPlane.Update(c.transport, c.policy)
}
