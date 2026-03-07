# Proxera Panel (Frontend)

User-facing panel for managing Proxera agents and proxy configurations.

## Quick Start

```bash
npm install
npm run dev
```

Visit http://localhost:5173

## Routes

- `/` - Auto-redirects to /login or /home
- `/login` - Sign in
- `/register` - Sign up  
- `/home` - Dashboard

## Environment

Uses parent `.env` file via symlink.
Required: `PUBLIC_API_URL`, `PUBLIC_WS_URL`
