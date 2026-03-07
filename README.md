# Proxera

Open-source, self-hosted multi-node nginx proxy manager with integrated DNS management, CrowdSec security, metrics, and alerting.

## Features

- **Multi-node management** - Manage nginx across multiple servers from a single panel
- **DNS management** - Multi-provider support (Cloudflare, IONOS, Porkbun)
- **CrowdSec integration** - Built-in security layer with decision and collection management
- **Metrics and alerting** - Traffic dashboards, geo maps, customizable alert rules
- **Certificate management** - Automated Let's Encrypt via DNS-01 challenges
- **Role-based access** - Admin, Member, and Viewer roles with resource scoping

## Architecture

Proxera uses a **Control Node + Agent** architecture:

- **Control Node**: Runs the backend API, web panel, and manages the database
- **Agents**: Lightweight binaries deployed on proxy servers, connect back via WebSocket

## Quick Start

### Prerequisites

- Docker and Docker Compose
- PostgreSQL 16+ with TimescaleDB (included in docker-compose)

### Using Docker Compose

1. Clone the repository:

   ```bash
   git clone https://github.com/Proxera-dev/proxera.git
   cd proxera
   ```

2. Copy and configure the environment file:

   ```bash
   cp .env .env.local
   # Edit .env.local with your settings - especially:
   #   JWT_SECRET (min 32 chars)
   #   SESSION_SECRET (min 32 chars)
   #   ENCRYPTION_KEY (64 hex chars)
   #   PUBLIC_API_URL, PUBLIC_WS_URL, PUBLIC_SITE_URL
   ```

3. Start the services:

   ```bash
   docker compose up -d
   ```

4. Open the panel at `http://localhost:5173` and register your admin account (first user is auto-admin).

### Installing an Agent

On each proxy server you want to manage:

1. Register an agent in the panel (Agents page)
2. Run the install script on the target server:
   ```bash
   export PROXERA_API_KEY=<your_api_key>
   curl -sSL http://your-control-node:8080/install.sh | sudo bash -s -- <agent_id> http://your-control-node:8080
   ```

## Configuration

All configuration is done via environment variables (`.env` file). Key settings:

| Variable                    | Description                                 |
| --------------------------- | ------------------------------------------- |
| `API_BASE_URL`              | Backend API URL                             |
| `PUBLIC_API_URL`            | URL the panel uses to reach the API         |
| `PUBLIC_WS_URL`             | WebSocket URL for the panel                 |
| `PUBLIC_SITE_URL`           | Panel's own URL                             |
| `AGENT_PANEL_URL`           | URL agents use to reach the control node    |
| `ENABLE_EMAIL_VERIFICATION` | Set to `true` to require email verification |
| `ENABLE_REGISTRATION`       | Set to `true` to allow open registration    |

## Auth Model

| Role   | Permissions                                                           |
| ------ | --------------------------------------------------------------------- |
| Admin  | Full access - manage users, assign resources, all configuration       |
| Member | Full access to assigned DNS providers and agents (cannot delete them) |
| Viewer | Read-only access to assigned DNS providers and agents                 |

- First registered user is automatically made admin
- Admins assign DNS providers and agents to members
- Registration mode configurable: open, invite-only, or disabled

## Tech Stack

- **Backend**: Go (Fiber framework)
- **Frontend**: SvelteKit
- **Database**: PostgreSQL + TimescaleDB
- **Agent**: Go

## Contributing

Contributions are welcome. Please sign the Contributor License Agreement (CLA) before submitting pull requests.

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## Support the Project

If you find Proxera useful, consider supporting its development:

- [GitHub Sponsors](https://github.com/sponsors/Proxera-dev)
- [Ko-fi](https://ko-fi.com/proxera)

## License

Proxera is licensed under the [GNU Affero General Public License v3.0](LICENSE).
