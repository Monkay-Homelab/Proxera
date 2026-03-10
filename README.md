# Proxera

Open-source, self-hosted reverse proxy manager for multi-node nginx deployments — with DNS management, SSL automation, CrowdSec security, real-time metrics, and alerting.

## Features

- **Multi-node proxy management** — deploy and manage nginx hosts across multiple servers from a single panel
- **DNS management** — Cloudflare, IONOS, Porkbun with DDNS and encrypted zone backups
- **SSL automation** — Let's Encrypt with DNS-01 challenge, wildcards, auto-renewal, and expiry alerts
- **CrowdSec integration** — install, manage decisions, collections, bouncers, and view blocked IPs by country/ASN
- **Real-time metrics** — request rate, latency percentiles, bandwidth, status codes, geo map, top visitors
- **Alerting** — 9 alert types across email, Discord, and webhook channels with cooldowns
- **Access control** — Admin, Member, and Viewer roles with per-user resource scoping and API key auth

## Architecture

```
┌─────────────────────────────┐
│       Control Node          │
│ ┌────────┐ ┌─────┐ ┌─────┐  │       ┌─────────┐
│ │  Panel │ │ API │ │Local│  │       │  Agent  │──── nginx
│ │(Svelte)│ │(Go) │ │Agent│  │  WS   │  (Go)   │
│ └────────┘ └──┬──┘ └──┬──┘  │◄─────►└─────────┘
│               │    ┌──┘─────│       ┌─────────┐
│            ┌──┴──┐ │nginx   │       │  Agent  │──── nginx
│            │ DB  │ │CrowdSec│       │  (Go)   │
│            │(PG) │ │        │  WS   │         │
│            └─────┘ └────────┘◄─────►└─────────┘
└─────────────────────────────┘
```

The **Control Node** is a single Go binary that embeds the SvelteKit panel, API server, and a local agent. **Agents** are lightweight binaries deployed on proxy servers that connect back via WebSocket.

## Quick Start

**Prerequisites:** Docker and Docker Compose

**1. Create a `.env` file** with the bare minimum:

```env
DB_PASSWORD=your-secure-database-password
JWT_SECRET=your-jwt-secret-min-32-chars
ENCRYPTION_KEY=your-64-hex-char-encryption-key
```

Generate secure values with `openssl rand -hex 32`.

**2. Create a `docker-compose.yml`:**

```yaml
services:
  proxera:
    image: monkayhomelab/proxera:latest
    restart: unless-stopped
    depends_on:
      postgres:
        condition: service_healthy
    env_file: .env
    environment:
      DB_HOST: postgres
      API_PORT: "5173"
      PROXERA_DOCKER: "true"
      NGINX_RELOAD_CMD: "docker kill -s HUP proxera-nginx-1"
      NGINX_TEST_CMD: "docker exec proxera-nginx-1 nginx -t"
      CROWDSEC_CONTAINER: "proxera-crowdsec-1"
    ports:
      - "5173:5173"
    volumes:
      - nginx-conf:/etc/nginx/conf.d
      - nginx-ssl:/etc/nginx/ssl
      - nginx-logs:/var/log/nginx
      - acme-challenge:/var/www/proxera-acme
      - proxera-data:/var/lib/proxera
      - agent-downloads:/app/downloads
      - /var/run/docker.sock:/var/run/docker.sock:ro
    group_add:
      - "${DOCKER_GID:-999}"

  postgres:
    image: timescale/timescaledb:2.17.2-pg16
    restart: unless-stopped
    environment:
      POSTGRES_DB: proxera
      POSTGRES_USER: proxera
      POSTGRES_PASSWORD: ${DB_PASSWORD}
    volumes:
      - pgdata:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U proxera"]
      interval: 5s
      timeout: 5s
      retries: 5

  nginx:
    image: nginx:1.27
    restart: unless-stopped
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - nginx-conf:/etc/nginx/conf.d
      - nginx-ssl:/etc/nginx/ssl:ro
      - nginx-logs:/var/log/nginx
      - acme-challenge:/var/www/proxera-acme:ro
    depends_on:
      - proxera

  crowdsec:
    image: crowdsecurity/crowdsec:v1.6.4
    restart: unless-stopped
    environment:
      COLLECTIONS: "crowdsecurity/nginx"
    volumes:
      - nginx-logs:/var/log/nginx:ro
      - crowdsec-config:/etc/crowdsec
      - crowdsec-data:/var/lib/crowdsec/data
    depends_on:
      - nginx

volumes:
  pgdata:
  nginx-conf:
  nginx-ssl:
  nginx-logs:
  acme-challenge:
  proxera-data:
  agent-downloads:
  crowdsec-config:
  crowdsec-data:
```

**3. Start it:**

```bash
docker compose up -d
```

Open `http://your-server:5173` and create your admin account. The first registered user is automatically admin.

### Adding Remote Agents

Register an agent in the panel, then run on each proxy server:

```bash
curl -sSL http://your-control-node:5173/install.sh | sudo PROXERA_API_KEY=<key> bash -s -- <agent_id> http://your-control-node:5173
```

See [`.env.example`](.env.example) for all available configuration options.

## Tech Stack

Go (Fiber) · SvelteKit · PostgreSQL + TimescaleDB · nginx · CrowdSec

## Contributing

Contributions are welcome. See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

[GNU Affero General Public License v3.0](LICENSE)
