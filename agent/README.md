# Proxera Agent — Source Transparency

This repository contains the source code for the Proxera agent binary. It is published so users can verify exactly what the agent does and what data it sends to the panel.

---

## What the Agent Does

The agent runs on your server and manages nginx reverse proxy configurations on behalf of the Proxera panel. It:

- Receives nginx configuration commands over a WebSocket connection and applies them locally
- Validates every config with `nginx -t` before applying, and auto-rolls back if nginx fails to reload
- Collects nginx access log metrics (request counts, status codes, latency, bytes transferred) and sends them to the panel on a configurable interval (default: every 5 minutes)
- Reports basic server info on connection: OS, architecture, agent version, and WAN IP address
- Sends a heartbeat every 60 seconds so the panel can show online/offline status

---

## What Data Is Collected

| Data | Why |
|------|-----|
| WAN IP address | Shown in the panel; used for DDNS if enabled |
| nginx access log metrics | Powers the traffic/metrics dashboard |
| nginx version | Shown in the panel |
| OS and architecture | Shown in the panel |
| Agent version | Used to prompt updates |
| CrowdSec decisions and alerts | Shown in the CrowdSec panel tab (only if CrowdSec is installed and you enable the integration) |

Metrics are aggregated from nginx access logs already present on your server. Raw log lines are never transmitted — only the aggregated counts per time bucket.

---

## What the Agent Does NOT Do

- It does not read files outside of nginx config directories and nginx access logs
- It does not collect passwords, environment variables, or application data
- It does not make outbound connections except to the panel URL you configured during registration and external IP lookup services (api.ipify.org / icanhazip.com / ifconfig.me) to detect your WAN IP
- It does not execute arbitrary commands — the only operations it performs are nginx config writes, nginx reloads, and CrowdSec management (if you enable it)
- It does not phone home to Proxera infrastructure by default — the panel URL is set by you during registration and stored in your local config file

---

## Key Files

| File | What it does |
|------|-------------|
| `cmd/proxera/main.go` | Entry point and CLI flags |
| `internal/client/websocket.go` | WebSocket connection, heartbeat, command handling |
| `internal/client/client.go` | Panel registration |
| `internal/metrics/collector.go` | nginx log parsing and metric aggregation |
| `internal/metrics/parser.go` | Log line format parsing |
| `internal/deploy/deploy.go` | Config application and rollback logic |
| `internal/nginx/generator.go` | nginx config generation from panel-supplied host definitions |
| `internal/crowdsec/` | CrowdSec integration (only active if installed and enabled) |

---

## Safety Features

- Every config change is validated with `nginx -t` before applying
- A backup is taken before every apply
- If nginx reload fails, the previous config is automatically restored
- The agent exits (rather than silently retrying) if the WebSocket connection drops, allowing systemd to restart it cleanly
