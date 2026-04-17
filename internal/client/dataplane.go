package client

import (
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"easyvpn/internal/config"
	"easyvpn/internal/control"
	"easyvpn/internal/transport"
	"easyvpn/internal/tun"
)

type DataPlane struct {
	cfg          config.ClientConfig
	peerID       string
	sessionToken string

	mu        sync.RWMutex
	transport control.TransportInfo
	policy    control.Policy
	current   *transport.FramedConn
	currentRW net.Conn
	tun       *tun.Device
	started   bool
}

func NewDataPlane(cfg config.ClientConfig, peerID, sessionToken string) *DataPlane {
	return &DataPlane{
		cfg:          cfg,
		peerID:       peerID,
		sessionToken: sessionToken,
	}
}

func (d *DataPlane) Start(transportInfo control.TransportInfo, policy control.Policy) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.transport = transportInfo
	d.policy = policy
	if d.started {
		return d.configureLocked()
	}

	device, err := tun.Open(d.cfg.TunnelName)
	if err != nil {
		return err
	}
	d.tun = device
	if err := d.configureLocked(); err != nil {
		_ = d.tun.Close()
		d.tun = nil
		return err
	}

	d.started = true
	go d.tunReadLoop()
	go d.connectionLoop()
	return nil
}

func (d *DataPlane) Update(transportInfo control.TransportInfo, policy control.Policy) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	dataAddrChanged := d.transport.DataAddr != transportInfo.DataAddr || d.transport.TLSEnabled != transportInfo.TLSEnabled
	d.transport = transportInfo
	d.policy = policy

	if !d.started {
		return fmt.Errorf("data plane is not started")
	}

	if err := d.configureLocked(); err != nil {
		return err
	}
	if dataAddrChanged && d.currentRW != nil {
		_ = d.currentRW.Close()
	}
	return nil
}

func (d *DataPlane) configureLocked() error {
	if d.tun == nil {
		return fmt.Errorf("TUN device is not initialized")
	}
	return d.tun.Configure(d.policy.Tunnel.ClientAddress, d.policy.Tunnel.MTU)
}

func (d *DataPlane) tunReadLoop() {
	buffer := make([]byte, 65535)
	for {
		n, err := d.tun.ReadPacket(buffer)
		if err != nil {
			log.Printf("data plane TUN read stopped: %v", err)
			return
		}
		packet := append([]byte(nil), buffer[:n]...)

		conn := d.getConn()
		if conn == nil {
			continue
		}
		if err := conn.WriteFrame(packet); err != nil {
			log.Printf("data plane send packet failed: %v", err)
			d.clearConn(conn)
		}
	}
}

func (d *DataPlane) connectionLoop() {
	backoff := time.Second
	for {
		snapshot := d.snapshot()
		if snapshot.transport.DataAddr == "" {
			log.Printf("data plane waiting for server data address")
			time.Sleep(backoff)
			continue
		}

		conn, framed, err := d.connect(snapshot.transport)
		if err != nil {
			log.Printf("data plane connect failed: %v", err)
			time.Sleep(backoff)
			if backoff < 10*time.Second {
				backoff *= 2
			}
			continue
		}

		backoff = time.Second
		d.setConn(conn, framed)
		log.Printf("data plane connected to %s tls=%v", snapshot.transport.DataAddr, snapshot.transport.TLSEnabled)

		if err := d.connReadLoop(framed); err != nil {
			log.Printf("data plane connection closed: %v", err)
		}
		d.clearConn(framed)
		time.Sleep(backoff)
	}
}

func (d *DataPlane) connReadLoop(conn *transport.FramedConn) error {
	for {
		packet, err := conn.ReadFrame()
		if err != nil {
			return err
		}
		if len(packet) == 0 {
			continue
		}
		if _, err := d.tun.WritePacket(packet); err != nil {
			return err
		}
	}
}

func (d *DataPlane) connect(transportInfo control.TransportInfo) (net.Conn, *transport.FramedConn, error) {
	conn, err := d.dialData(transportInfo)
	if err != nil {
		return nil, nil, err
	}

	framed := transport.NewFramedConn(conn)
	if err := framed.WriteJSON(control.DataConnectRequest{
		SessionToken: d.sessionToken,
		PeerID:       d.peerID,
	}); err != nil {
		_ = conn.Close()
		return nil, nil, err
	}

	var resp control.DataConnectResponse
	if err := framed.ReadJSON(&resp); err != nil {
		_ = conn.Close()
		return nil, nil, err
	}
	if resp.Message != "" && resp.Message != "ok" {
		_ = conn.Close()
		return nil, nil, fmt.Errorf("data handshake rejected: %s", resp.Message)
	}
	if resp.PeerID != d.peerID {
		_ = conn.Close()
		return nil, nil, fmt.Errorf("unexpected peer id in data handshake: %s", resp.PeerID)
	}
	return conn, framed, nil
}

func (d *DataPlane) dialData(transportInfo control.TransportInfo) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	if !transportInfo.TLSEnabled {
		return dialer.Dial("tcp", transportInfo.DataAddr)
	}

	tlsConfig, err := transport.LoadClientTLSConfig(
		d.cfg.DataTLSCAFile,
		d.cfg.DataTLSServerName,
		d.cfg.DataTLSInsecureSkipVerify,
	)
	if err != nil {
		return nil, err
	}

	return tls.DialWithDialer(dialer, "tcp", transportInfo.DataAddr, tlsConfig)
}

func (d *DataPlane) getConn() *transport.FramedConn {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.current
}

func (d *DataPlane) setConn(raw net.Conn, conn *transport.FramedConn) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.currentRW = raw
	d.current = conn
}

func (d *DataPlane) clearConn(conn *transport.FramedConn) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.current != conn {
		return
	}
	if d.currentRW != nil {
		_ = d.currentRW.Close()
	}
	d.current = nil
	d.currentRW = nil
}

type dataPlaneSnapshot struct {
	transport control.TransportInfo
}

func (d *DataPlane) snapshot() dataPlaneSnapshot {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return dataPlaneSnapshot{
		transport: d.transport,
	}
}
