# Contributing to Proxera

We welcome contributions from the community.

## Before You Start

- Read and agree to the [Contributor License Agreement](CLA.md)
- Check existing issues and pull requests to avoid duplicate work

## Development Setup

1. Clone the repo
2. Copy `.env` and adjust settings for your local environment
3. Start PostgreSQL with TimescaleDB (or use `docker compose up postgres`)
4. Build and run the backend: `cd backend && go build -o server ./cmd/server && ./server`
5. Build and run the panel: `cd panel && npm install && npm run dev`

## Pull Requests

- Keep PRs focused on a single change
- Include a clear description of what and why
- Ensure existing functionality is not broken
- Follow the existing code style

## Reporting Issues

Open an issue on GitHub with:
- Steps to reproduce
- Expected vs actual behavior
- Environment details (OS, browser, Go version)
