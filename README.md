# easyVpn

`easyVpn` is a lightweight self-hosted private tunnel prototype written in Go.

This repository currently focuses on:

- a simple control plane
- a web admin UI
- client login and heartbeats
- policy delivery for split tunnel and full tunnel modes

It does not yet implement traffic obfuscation or censorship-evasion behavior.

## Architecture

- `cmd/easyvpn-server`: control server and web console
- `cmd/easyvpn-client`: client agent
- `internal/config`: config loading
- `internal/control`: shared API types and policy evaluation
- `internal/server`: server implementation
- `internal/client`: client implementation

## Features in this prototype

- JSON config files
- account/password login
- in-memory peer management
- per-user policy management
- web console
- REST API for users, peers, and policies
- route policy model
- route plan generation and Linux route sync
- Linux TUN data plane scaffold for client/server packet bridging

## Quick start

### 1. Create server config

```json
{
  "listen": ":8443",
  "admin_bind": ":8080",
  "admin_token": "change-me",
  "enable_tunnel": false,
  "public_data_addr": "127.0.0.1:8443",
  "tunnel_name": "easyvpn0",
  "tunnel_address": "10.200.0.1/24",
  "tunnel_mtu": 1380,
  "users": [
    {
      "username": "demo",
      "password": "demo123",
      "policy": {
        "mode": "split",
        "routes": ["10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"],
        "dns": ["1.1.1.1", "8.8.8.8"],
        "tunnel": {
          "client_address": "10.200.0.2/24",
          "server_address": "10.200.0.1",
          "mtu": 1380
        }
      }
    }
  ]
}
```

Save as `server.json`, or just use `examples/server.json`.

### 2. Start server

```bash
go run ./cmd/easyvpn-server -config ./server.json
```

Open `http://127.0.0.1:8080` in a browser.

### 3. Create client config

```json
{
  "server_url": "http://127.0.0.1:8080",
  "username": "demo",
  "password": "demo123",
  "node_name": "my-laptop",
  "enable_tunnel": false,
  "tunnel_name": "easyvpn0",
  "apply_system_routes": false
}
```

Save as `client.json`, or just use `examples/client.json`.

### 4. Start client

```bash
go run ./cmd/easyvpn-client -config ./client.json
```

The client will log in with account/password, fetch the user's policy from the server, maintain heartbeats, and build a route plan. If `enable_tunnel` is enabled on both sides, it will also start a Linux TUN-backed packet bridge over the data listener.

## Route policy behavior

- `mode = "full"`: all traffic is marked to use the server tunnel
- `mode = "split"`: only destinations matching configured CIDR routes use the tunnel

Examples:

- `10.0.0.0/8`
- `172.16.0.0/12`
- `192.168.0.0/16`

## Tunnel fields

- `tunnel.client_address`: client-side tunnel IP and prefix, for example `10.200.0.2/24`
- `tunnel.server_address`: server-side tunnel IP, for example `10.200.0.1`
- `tunnel.mtu`: target tunnel MTU

## Route application

- By default, the client only logs the generated route plan.
- When `apply_system_routes` is `true`, Linux clients will execute `ip route replace` to sync split/full tunnel routes.
- In `full` mode, automatic route application currently requires the advertised data endpoint to use a literal IP address, so the server connection can be kept outside the tunnel.

## Data plane

- `enable_tunnel = true` on the server starts a TCP data listener on `listen` and creates a Linux TUN interface with `tunnel_name` and `tunnel_address`.
- `enable_tunnel = true` on the client creates its Linux TUN interface, authenticates the data connection with the login session token, and forwards raw IP packets over the TCP tunnel.
- The server writes client packets into the server TUN device and routes return packets back to the matching client by destination tunnel IP.
- This is the minimum user-space L3 tunnel path; it still depends on Linux network configuration outside the Go process.

## Current limitation

The current tunnel path is Linux-only and uses a simple TCP packet stream. It does not yet provide TLS, multiplexing, persistence, automatic IP allocation, or automatic server-side NAT/firewall setup.

## Linux notes

- Running the client or server with `enable_tunnel = true` requires Linux plus privileges to create and configure `/dev/net/tun`.
- For internet egress through the server, you still need Linux IP forwarding and NAT rules on the server host.

## Next steps

The current codebase intentionally separates control plane and data plane so that we can later add:

- TLS or Noise-style transport security
- automatic IP allocation
- automatic server-side NAT setup
- persistent storage
- peer key management
- richer route application and DNS handling

## Safety boundary

This project is positioned as a private networking tool for legitimate self-hosted access. It intentionally does not include stealth, fingerprint-masking, or censorship-evasion mechanisms.
