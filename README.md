# easyVpn

`easyVpn` is a lightweight self-hosted private tunnel prototype written in Go.

This repository currently focuses on:

- a simple control plane
- a web admin UI
- client enrollment and heartbeats
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
- enrollment tokens
- in-memory peer management
- web console
- REST API for peers and policies
- route policy model
- local route decision demo command

## Quick start

### 1. Create server config

```json
{
  "listen": ":8443",
  "admin_bind": ":8080",
  "admin_token": "change-me",
  "enrollment_tokens": ["demo-token"]
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
  "enrollment_token": "demo-token",
  "node_name": "my-laptop"
}
```

Save as `client.json`, or just use `examples/client.json`.

### 4. Start client

```bash
go run ./cmd/easyvpn-client -config ./client.json
```

The client will register, poll policy, and print route decisions.

## Route policy behavior

- `mode = "full"`: all traffic is marked to use the server tunnel
- `mode = "split"`: only destinations matching configured CIDR routes use the tunnel

Examples:

- `10.0.0.0/8`
- `172.16.0.0/12`
- `192.168.0.0/16`

## Current limitation

The current client prints route decisions and keeps policy in sync, but it does not yet create a system TUN interface or push packets through a real encrypted data path.

## Next steps

The current codebase intentionally separates control plane and data plane so that we can later add:

- TUN device support
- packet forwarding
- NAT on the server side
- persistent storage
- user authentication
- peer key management
- Linux route application

## Safety boundary

This project is positioned as a private networking tool for legitimate self-hosted access. It intentionally does not include stealth, fingerprint-masking, or censorship-evasion mechanisms.
