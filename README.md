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
DB_USER=custom-user-for-database
DB_NAME=custom-table-name
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
    container_name: proxera
    restart: unless-stopped
    depends_on:
      postgres:
        condition: service_healthy
    environment:
      DB_HOST: proxera-postgres
      DB_PORT: "5432"
      DB_USER: ${DB_USER:-proxera}
      DB_NAME: ${DB_NAME:-proxera}
      DB_PASSWORD: ${DB_PASSWORD}
      DB_SSL_MODE: disable
      JWT_SECRET: ${JWT_SECRET}
      ENCRYPTION_KEY: ${ENCRYPTION_KEY}
      API_PORT: "5173"
      PROXERA_DOCKER: "true"
      NGINX_RELOAD_CMD: "docker kill -s HUP proxera-nginx"
      NGINX_TEST_CMD: "docker exec proxera-nginx nginx -t"
      CROWDSEC_CONTAINER: "proxera-crowdsec"
    ports:
      - "${PROXERA_PORT:-5173}:5173"
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
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:5173/health"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 10s
    stop_grace_period: 30s

  postgres:
    image: timescale/timescaledb:2.17.2-pg16
    container_name: proxera-postgres
    restart: unless-stopped
    environment:
      POSTGRES_DB: ${DB_NAME:-proxera}
      POSTGRES_USER: ${DB_USER:-proxera}
      POSTGRES_PASSWORD: ${DB_PASSWORD}
    volumes:
      - pgdata:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U ${DB_USER:-proxera}"]
      interval: 5s
      timeout: 5s
      retries: 5

  nginx:
    image: nginx:1.27
    container_name: proxera-nginx
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
    healthcheck:
      test: ["CMD-SHELL", "curl -f http://localhost/ || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 5s

  crowdsec:
    image: crowdsecurity/crowdsec:v1.6.4
    container_name: proxera-crowdsec
    restart: unless-stopped
    environment:
      COLLECTIONS: "crowdsecurity/nginx"
    volumes:
      - nginx-logs:/var/log/nginx:ro
      - crowdsec-config:/etc/crowdsec
      - crowdsec-data:/var/lib/crowdsec/data
    depends_on:
      - nginx
    healthcheck:
      test: ["CMD", "cscli", "version"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 15s

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
