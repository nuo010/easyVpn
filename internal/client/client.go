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
	cfg        config.ClientConfig
	httpClient *http.Client
	peerID     string
	policy     control.Policy
}

func New(cfg config.ClientConfig) *Client {
	return &Client{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (c *Client) Run() error {
	if err := c.register(); err != nil {
		return err
	}
	log.Printf("registered as peer %s", c.peerID)
	c.printPolicy()

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

func (c *Client) register() error {
	resp := control.RegisterResponse{}
	if err := c.post("/api/v1/register", control.RegisterRequest{
		Token:    c.cfg.EnrollmentToken,
		NodeName: c.cfg.NodeName,
	}, &resp); err != nil {
		return fmt.Errorf("register: %w", err)
	}
	c.peerID = resp.PeerID
	c.policy = resp.Policy
	return nil
}

func (c *Client) heartbeat() error {
	var resp struct {
		PeerID string         `json:"peer_id"`
		Policy control.Policy `json:"policy"`
	}

	if err := c.post("/api/v1/heartbeat", control.HeartbeatRequest{
		PeerID:      c.peerID,
		Version:     version,
		ReportedAt:  time.Now().UTC(),
		LocalIPHint: firstNonLoopbackIPv4(),
	}, &resp); err != nil {
		return err
	}

	c.policy = resp.Policy
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
	log.Printf("policy mode=%s routes=%v dns=%v", c.policy.Mode, c.policy.Routes, c.policy.DNS)
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
