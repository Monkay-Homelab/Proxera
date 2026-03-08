# Proxera

Open-source, self-hosted multi-node reverse proxy manager built on nginx — with integrated DNS management, SSL automation, CrowdSec security, real-time metrics, and alerting.

## Features

### Reverse Proxy Management
- **Multi-node architecture** — manage nginx across multiple servers from a single panel
- **Host configuration** — deploy, update, and remove proxy hosts with advanced options (custom headers, rate limiting, basic auth, upstream SSL)
- **Backups** — automatic nginx config backups with one-click restore

### DNS Management
- **Multi-provider support** — Cloudflare, IONOS, Porkbun
- **Full record management** — A, AAAA, CNAME, MX, NS, TXT, SRV, and more
- **Dynamic DNS (DDNS)** — auto-update records when agent WAN IP changes
- **Export/Import** — password-encrypted DNS zone backups (AES-256-GCM + Argon2)

### SSL Certificates
- **Automated issuance** via Let's Encrypt (DNS-01 challenge)
- **Wildcard support** with automatic root domain inclusion
- **Auto-renewal** with configurable expiry alerts
- **Multi-domain SANs** — multiple domains per certificate
- **Staging mode** for testing without rate limits

### CrowdSec Integration
- **Install/uninstall** CrowdSec per agent with enrollment key
- **Decision management** — list, ban, and unban IPs with configurable duration
- **Collections and bouncers** — install and manage from the panel
- **IP whitelisting** — exempt trusted addresses
- **Blocked IP dashboard** — view bans by IP, country, ASN, and scenario

### Metrics & Monitoring
- **Real-time dashboards** — request rate, latency (P50/P95/P99), bandwidth, status codes, cache hits
- **Choropleth world map** — visitor geolocation by country
- **Top visitors** — ranked by request count with geo enrichment
- **Time ranges** — 1h, 6h, 12h, 24h, 7d, 30d, 90d, all
- **Per-agent and per-domain filtering**
- **Auto-refresh** with 30-second intervals
- **TimescaleDB** continuous aggregates for efficient long-range queries

### Alerting System
- **9 alert types** — agent offline, cert expiry, cert renewal failed, high latency, traffic spike, error rate, host down, bandwidth threshold, CrowdSec ban
- **3 notification channels** — Email (SMTP), Discord webhook, custom HTTP webhook
- **Cooldown periods** to prevent alert fatigue
- **Alert history** with filtering and manual resolution
- **Quick setup** — one-click default rules

### Access Control
- **Role-based permissions** — Admin, Member, Viewer
- **Resource scoping** — admins assign agents and DNS providers to users
- **API key authentication** — `pxk_` prefixed keys with configurable expiration
- **Registration modes** — open, invite-only, or closed

## Architecture

Proxera uses a **Control Node + Agent** model:

- **Control Node** — all-in-one Go binary embedding the API server, SvelteKit panel, and a local agent
- **Agents** — lightweight Go binaries on proxy servers, connected via WebSocket
- **Database** — PostgreSQL with TimescaleDB for metrics storage and aggregation

```
┌─────────────────────────────┐
│       Control Node          │
│  ┌───────┐ ┌─────┐ ┌─────┐ │       ┌─────────┐
│  │ Panel │ │ API │ │Local│ │       │  Agent  │──── nginx
│  │(Svelte)│ │(Go) │ │Agent│ │  WS   │  (Go)   │
│  └───────┘ └──┬──┘ └──┬──┘ │◄─────►└─────────┘
│               │    ┌───┘    │       ┌─────────┐
│            ┌──┴──┐ │nginx  │       │  Agent  │──── nginx
│            │ DB  │ │CrowdSec│       │  (Go)   │
│            │(PG) │ │       │  WS   │         │
│            └─────┘ └───────┘◄─────►└─────────┘
└─────────────────────────────┘
```

## Quick Start

### Prerequisites

- Docker and Docker Compose

### 1. Clone and configure

```bash
git clone https://github.com/Monkay-Homelab/Proxera.git
cd Proxera
cp .env .env.local
```

Edit `.env.local` with your settings:

```bash
# Required secrets (generate with: openssl rand -hex 32)
JWT_SECRET=<min 32 chars>
SESSION_SECRET=<min 32 chars>
ENCRYPTION_KEY=<64 hex chars>

# Public URLs (used by panel and agents)
PUBLIC_API_URL=http://your-server:5173
PUBLIC_WS_URL=ws://your-server:5173
PUBLIC_SITE_URL=http://your-server:5173
```

### 2. Start services

```bash
docker compose up -d
```

This starts the full stack: Proxera control node, PostgreSQL + TimescaleDB, nginx, and CrowdSec.

### 3. Register

Open `http://your-server:5173` and create your admin account. The first registered user is automatically made admin.

### Installing Agents

On each proxy server you want to manage:

1. Register an agent in the panel (Agents page)
2. Run the install script:

```bash
export PROXERA_API_KEY=<your_api_key>
curl -sSL http://your-control-node:5173/install.sh | sudo bash -s -- <agent_id> http://your-control-node:5173
```

The agent auto-detects OS/architecture, installs as a systemd service, and connects back via WebSocket. Agents support self-update from the panel.

## Configuration

### Environment Variables

Core settings are in `.env`. Many can also be changed at runtime in **Admin > Settings**.

| Variable | Description |
|---|---|
| `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME` | PostgreSQL connection |
| `API_HOST`, `API_PORT` | Server bind address |
| `JWT_SECRET` | JWT signing key |
| `ENCRYPTION_KEY` | Field-level encryption for credentials |
| `PUBLIC_API_URL` | API URL used by the panel |
| `PUBLIC_WS_URL` | WebSocket URL used by the panel |
| `PUBLIC_SITE_URL` | Panel URL (used in emails and alerts) |
| `SMTP_HOST`, `SMTP_PORT`, `SMTP_USER`, `SMTP_PASSWORD`, `SMTP_FROM` | Email notifications |
| `ENABLE_EMAIL_VERIFICATION` | Require email verification for new accounts |
| `ENABLE_REGISTRATION` | Allow open registration |

### Runtime Settings (Admin Panel)

- Registration mode (open / invite-only / closed)
- Invite code
- Public URLs
- SMTP configuration
- Email verification toggle
- ACME staging toggle

## Auth Model

| Role | Permissions |
|---|---|
| Admin | Full access — manage users, assign resources, all configuration |
| Member | Full access to assigned DNS providers and agents (cannot delete) |
| Viewer | Read-only access to assigned DNS providers and agents |

API keys inherit the creating user's role and support configurable expiration (30d, 90d, 365d, never).

## Tech Stack

- **Backend**: Go (Fiber)
- **Frontend**: SvelteKit
- **Database**: PostgreSQL + TimescaleDB
- **Agent**: Go
- **Proxy**: nginx
- **Security**: CrowdSec

## Building from Source

```bash
# Build panel
cd panel && npm run build

# Build control node (embeds panel)
cp -r panel/build backend/cmd/control/panel
VERSION=$(git describe --tags --always --dirty)
cd backend && go build -ldflags "-X github.com/proxera/agent/pkg/version.Version=$VERSION" -o proxera-control ./cmd/control/main.go

# Build agent
cd agent && go build -ldflags "-X github.com/proxera/agent/pkg/version.Version=$VERSION" -o bin/proxera-linux-amd64 cmd/proxera/main.go

# Or build everything via Docker
VERSION=$(git describe --tags --always --dirty) docker compose build
```

## Contributing

Contributions are welcome. Please sign the Contributor License Agreement (CLA) before submitting pull requests.

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

Proxera is licensed under the [GNU Affero General Public License v3.0](LICENSE).
