package server

import (
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/netip"
	"sync"

	"easyvpn/internal/config"
	"easyvpn/internal/control"
	"easyvpn/internal/packet"
	"easyvpn/internal/transport"
	"easyvpn/internal/tun"
)

type DataPlane struct {
	cfg   config.ServerConfig
	store *Store
	tun   *tun.Device

	mu      sync.RWMutex
	clients map[string]*serverClientConn
}

type serverClientConn struct {
	peerID   string
	clientIP string
	raw      net.Conn
	framed   *transport.FramedConn
}

func NewDataPlane(cfg config.ServerConfig, store *Store) *DataPlane {
	return &DataPlane{
		cfg:     cfg,
		store:   store,
		clients: make(map[string]*serverClientConn),
	}
}

func (d *DataPlane) Start() error {
	device, err := tun.Open(d.cfg.TunnelName)
	if err != nil {
		return err
	}
	if err := device.Configure(d.cfg.TunnelAddress, d.cfg.TunnelMTU); err != nil {
		_ = device.Close()
		return err
	}
	d.tun = device

	listener, err := d.listen()
	if err != nil {
		_ = device.Close()
		return err
	}

	go d.tunReadLoop()
	go d.acceptLoop(listener)
	return nil
}

func (d *DataPlane) acceptLoop(listener net.Listener) {
	defer listener.Close()

	log.Printf("easyVpn data plane listening on %s tls=%v", d.cfg.Listen, d.cfg.DataTLSEnabled)
	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("data plane accept failed: %v", err)
			continue
		}
		go d.handleConn(conn)
	}
}

func (d *DataPlane) listen() (net.Listener, error) {
	if !d.cfg.DataTLSEnabled {
		return net.Listen("tcp", d.cfg.Listen)
	}

	tlsConfig, err := transport.LoadServerTLSConfig(d.cfg.DataTLSCertFile, d.cfg.DataTLSKeyFile)
	if err != nil {
		return nil, err
	}
	return tls.Listen("tcp", d.cfg.Listen, tlsConfig)
}

func (d *DataPlane) handleConn(raw net.Conn) {
	framed := transport.NewFramedConn(raw)

	var req control.DataConnectRequest
	if err := framed.ReadJSON(&req); err != nil {
		log.Printf("data plane handshake read failed: %v", err)
		_ = raw.Close()
		return
	}

	peer, ok := d.store.ResolveSession(req.SessionToken)
	if !ok || (req.PeerID != "" && req.PeerID != peer.ID) {
		_ = framed.WriteJSON(control.DataConnectResponse{Message: "invalid session"})
		_ = raw.Close()
		return
	}

	clientIP, err := clientTunnelIP(peer.Policy)
	if err != nil {
		_ = framed.WriteJSON(control.DataConnectResponse{
			PeerID:  peer.ID,
			Message: err.Error(),
		})
		_ = raw.Close()
		return
	}

	conn := &serverClientConn{
		peerID:   peer.ID,
		clientIP: clientIP.String(),
		raw:      raw,
		framed:   framed,
	}
	d.registerClient(conn)
	defer d.unregisterClient(conn)

	if err := framed.WriteJSON(control.DataConnectResponse{
		PeerID:  peer.ID,
		Message: "ok",
	}); err != nil {
		log.Printf("data plane handshake write failed: %v", err)
		return
	}

	log.Printf("data plane session attached peer=%s client_ip=%s", peer.ID, clientIP)
	for {
		packetBytes, err := framed.ReadFrame()
		if err != nil {
			log.Printf("data plane read failed for peer %s: %v", peer.ID, err)
			return
		}
		if len(packetBytes) == 0 {
			continue
		}
		if _, err := d.tun.WritePacket(packetBytes); err != nil {
			log.Printf("data plane TUN write failed for peer %s: %v", peer.ID, err)
			return
		}
	}
}

func (d *DataPlane) tunReadLoop() {
	buffer := make([]byte, 65535)
	for {
		n, err := d.tun.ReadPacket(buffer)
		if err != nil {
			log.Printf("data plane TUN read stopped: %v", err)
			return
		}
		packetBytes := append([]byte(nil), buffer[:n]...)

		destination, err := packet.Destination(packetBytes)
		if err != nil {
			log.Printf("data plane drop packet: %v", err)
			continue
		}

		client := d.lookupClient(destination.String())
		if client == nil {
			continue
		}
		if err := client.framed.WriteFrame(packetBytes); err != nil {
			log.Printf("data plane forward failed for peer %s: %v", client.peerID, err)
			d.unregisterClient(client)
		}
	}
}

func (d *DataPlane) registerClient(client *serverClientConn) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if existing, ok := d.clients[client.clientIP]; ok && existing.raw != nil {
		_ = existing.raw.Close()
	}
	d.clients[client.clientIP] = client
}

func (d *DataPlane) unregisterClient(client *serverClientConn) {
	d.mu.Lock()
	defer d.mu.Unlock()

	existing, ok := d.clients[client.clientIP]
	if !ok || existing != client {
		return
	}
	delete(d.clients, client.clientIP)
	if client.raw != nil {
		_ = client.raw.Close()
	}
}

func (d *DataPlane) lookupClient(ip string) *serverClientConn {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.clients[ip]
}

func clientTunnelIP(policy control.Policy) (netip.Addr, error) {
	prefix, err := netip.ParsePrefix(policy.Tunnel.ClientAddress)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("invalid client tunnel address %q: %w", policy.Tunnel.ClientAddress, err)
	}
	return prefix.Addr(), nil
}
