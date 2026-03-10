package database

import (
	"context"
	"fmt"
	"log"

	"github.com/jackc/pgx/v5"
)

// Migration represents a single database migration with a numeric ID and a
// function that executes the DDL/DML inside a transaction.
type Migration struct {
	ID   int
	Name string
	Fn   func(tx pgx.Tx) error
}

// migrations is the ordered list of all schema migrations. New migrations are
// appended at the end with the next sequential ID. Each function receives a
// transaction — if it returns an error the transaction is rolled back and
// startup is aborted.
var migrations = []Migration{
	{
		ID:   1,
		Name: "initial_schema",
		Fn:   migration001InitialSchema,
	},
}

// RunMigrations ensures the schema_migrations tracking table exists, then
// applies every migration that has not yet been recorded, in order.
func RunMigrations() error {
	ctx := context.Background()

	// Create the tracking table if it doesn't exist.
	_, err := DB.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			id INTEGER PRIMARY KEY,
			name VARCHAR(255) NOT NULL,
			applied_at TIMESTAMPTZ DEFAULT NOW()
		);
	`)
	if err != nil {
		return fmt.Errorf("failed to create schema_migrations table: %w", err)
	}

	for _, m := range migrations {
		// Check whether this migration has already been applied.
		var exists bool
		err := DB.QueryRow(ctx,
			"SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE id = $1)", m.ID,
		).Scan(&exists)
		if err != nil {
			return fmt.Errorf("failed to check migration %d (%s): %w", m.ID, m.Name, err)
		}
		if exists {
			continue
		}

		log.Printf("Applying migration %03d_%s ...", m.ID, m.Name)

		tx, err := DB.Begin(ctx)
		if err != nil {
			return fmt.Errorf("failed to begin transaction for migration %d: %w", m.ID, err)
		}

		if err := m.Fn(tx); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("migration %d (%s) failed: %w", m.ID, m.Name, err)
		}

		// Record the migration as applied inside the same transaction.
		_, err = tx.Exec(ctx,
			"INSERT INTO schema_migrations (id, name) VALUES ($1, $2)", m.ID, m.Name,
		)
		if err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("failed to record migration %d: %w", m.ID, err)
		}

		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("failed to commit migration %d: %w", m.ID, err)
		}

		log.Printf("Migration %03d_%s applied successfully", m.ID, m.Name)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Migration 001 — Initial schema
// ---------------------------------------------------------------------------

func migration001InitialSchema(tx pgx.Tx) error {
	ctx := context.Background()

	query := `
		CREATE TABLE IF NOT EXISTS users (
			id SERIAL PRIMARY KEY,
			email VARCHAR(255) UNIQUE NOT NULL,
			name VARCHAR(255) NOT NULL,
			password VARCHAR(255) NOT NULL,
			role VARCHAR(50) DEFAULT 'member',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);

		CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);

		CREATE TABLE IF NOT EXISTS agents (
			id SERIAL PRIMARY KEY,
			user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			agent_id VARCHAR(255) UNIQUE NOT NULL,
			name VARCHAR(255) NOT NULL,
			api_key VARCHAR(255) UNIQUE NOT NULL,
			status VARCHAR(50) DEFAULT 'offline',
			version VARCHAR(50),
			os VARCHAR(50),
			arch VARCHAR(50),
			last_seen TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			ip_address VARCHAR(50),
			host_count INTEGER DEFAULT 0,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);

		CREATE INDEX IF NOT EXISTS idx_agents_user_id ON agents(user_id);
		CREATE INDEX IF NOT EXISTS idx_agents_agent_id ON agents(agent_id);
		CREATE INDEX IF NOT EXISTS idx_agents_api_key ON agents(api_key);

		-- Add LAN and WAN IP columns if they don't exist
		ALTER TABLE agents ADD COLUMN IF NOT EXISTS lan_ip VARCHAR(50);
		ALTER TABLE agents ADD COLUMN IF NOT EXISTS wan_ip VARCHAR(50);

		-- Add CrowdSec installed flag
		ALTER TABLE agents ADD COLUMN IF NOT EXISTS crowdsec_installed BOOLEAN DEFAULT false;

		-- Add nginx version tracking
		ALTER TABLE agents ADD COLUMN IF NOT EXISTS nginx_version VARCHAR(50) DEFAULT '';

		-- API key hash for secure storage (SHA-256)
		ALTER TABLE agents ADD COLUMN IF NOT EXISTS api_key_hash VARCHAR(64);
		CREATE INDEX IF NOT EXISTS idx_agents_api_key_hash ON agents(api_key_hash);

		-- Metrics collection interval
		ALTER TABLE agents ADD COLUMN IF NOT EXISTS metrics_interval INTEGER DEFAULT 300;

		-- Local agent flag (Control Node AIO)
		ALTER TABLE agents ADD COLUMN IF NOT EXISTS is_local BOOLEAN DEFAULT false;

		-- Last heartbeat timestamp
		ALTER TABLE agents ADD COLUMN IF NOT EXISTS last_heartbeat TIMESTAMP DEFAULT CURRENT_TIMESTAMP;

		CREATE TABLE IF NOT EXISTS dns_providers (
			id SERIAL PRIMARY KEY,
			user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			provider VARCHAR(50) NOT NULL,
			zone_id TEXT NOT NULL,
			api_key TEXT NOT NULL,
			domain VARCHAR(255),
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);

		CREATE INDEX IF NOT EXISTS idx_dns_providers_user_id ON dns_providers(user_id);

		CREATE TABLE IF NOT EXISTS dns_records (
			id SERIAL PRIMARY KEY,
			dns_provider_id INTEGER NOT NULL REFERENCES dns_providers(id) ON DELETE CASCADE,
			cf_record_id TEXT NOT NULL,
			record_type VARCHAR(50) NOT NULL,
			name VARCHAR(255) NOT NULL,
			content VARCHAR(1024) NOT NULL,
			ttl INTEGER DEFAULT 1,
			proxied BOOLEAN DEFAULT false,
			last_synced TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);

		CREATE INDEX IF NOT EXISTS idx_dns_records_provider_id ON dns_records(dns_provider_id);

		-- Add agent_id column to dns_records for DDNS agent assignment
		ALTER TABLE dns_records ADD COLUMN IF NOT EXISTS agent_id INTEGER REFERENCES agents(id) ON DELETE SET NULL;
		CREATE INDEX IF NOT EXISTS idx_dns_records_agent_id ON dns_records(agent_id);

		-- Unique constraint for upsert support during sync (original Cloudflare-specific)
		CREATE UNIQUE INDEX IF NOT EXISTS idx_dns_records_provider_cf ON dns_records(dns_provider_id, cf_record_id);

		-- Multi-provider support: provider_record_id replaces cf_record_id as the canonical remote ID.
		-- cf_record_id is kept for backward compatibility; new code writes provider_record_id.
		ALTER TABLE dns_providers ADD COLUMN IF NOT EXISTS api_secret TEXT;
		ALTER TABLE dns_records ADD COLUMN IF NOT EXISTS provider_record_id TEXT;
		-- Widen encrypted credential columns to TEXT (encrypted values can exceed 255 chars)
		ALTER TABLE dns_providers ALTER COLUMN zone_id TYPE TEXT;
		ALTER TABLE dns_providers ALTER COLUMN api_key TYPE TEXT;
		ALTER TABLE dns_providers ALTER COLUMN api_secret TYPE TEXT;
		ALTER TABLE dns_records ALTER COLUMN cf_record_id TYPE TEXT;
		ALTER TABLE dns_records ALTER COLUMN provider_record_id TYPE TEXT;
		UPDATE dns_records SET provider_record_id = cf_record_id WHERE provider_record_id IS NULL;
		CREATE UNIQUE INDEX IF NOT EXISTS idx_dns_records_provider_remote ON dns_records(dns_provider_id, provider_record_id);

		CREATE TABLE IF NOT EXISTS hosts (
			id SERIAL PRIMARY KEY,
			user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			provider_id INTEGER NOT NULL REFERENCES dns_providers(id) ON DELETE CASCADE,
			domain VARCHAR(255) NOT NULL,
			upstream_url VARCHAR(1024) NOT NULL,
			ssl BOOLEAN DEFAULT false,
			websocket BOOLEAN DEFAULT false,
			agent_id INTEGER REFERENCES agents(id) ON DELETE SET NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);

		CREATE INDEX IF NOT EXISTS idx_hosts_user_id ON hosts(user_id);
		CREATE INDEX IF NOT EXISTS idx_hosts_provider_id ON hosts(provider_id);
		CREATE INDEX IF NOT EXISTS idx_hosts_agent_id ON hosts(agent_id);

		-- Advanced host configuration (JSONB)
		ALTER TABLE hosts ADD COLUMN IF NOT EXISTS config JSONB DEFAULT '{}';

		CREATE TABLE IF NOT EXISTS certificates (
			id SERIAL PRIMARY KEY,
			user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			provider_id INTEGER NOT NULL REFERENCES dns_providers(id) ON DELETE CASCADE,
			domain VARCHAR(255) NOT NULL,
			san TEXT,
			certificate_pem TEXT,
			private_key_pem TEXT,
			issuer_pem TEXT,
			cert_url TEXT,
			status VARCHAR(20) DEFAULT 'pending',
			challenge_type VARCHAR(20) DEFAULT 'dns-01',
			issued_at TIMESTAMP,
			expires_at TIMESTAMP,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_certificates_user_id ON certificates(user_id);
		CREATE INDEX IF NOT EXISTS idx_certificates_provider_id ON certificates(provider_id);

		-- FK to certificates (must come after certificates table creation)
		ALTER TABLE hosts ADD COLUMN IF NOT EXISTS certificate_id INTEGER REFERENCES certificates(id) ON DELETE SET NULL;

		ALTER TABLE users ADD COLUMN IF NOT EXISTS acme_key TEXT;

		-- Email verification
		ALTER TABLE users ADD COLUMN IF NOT EXISTS email_verified BOOLEAN DEFAULT false;
		ALTER TABLE users ADD COLUMN IF NOT EXISTS verification_token VARCHAR(255);
		ALTER TABLE users ADD COLUMN IF NOT EXISTS verification_token_expires TIMESTAMP;

		-- Metrics hypertable (TimescaleDB)
		CREATE TABLE IF NOT EXISTS metrics (
			time TIMESTAMPTZ NOT NULL,
			agent_id VARCHAR(255) NOT NULL,
			domain VARCHAR(255) NOT NULL,
			request_count BIGINT DEFAULT 0,
			bytes_sent BIGINT DEFAULT 0,
			bytes_received BIGINT DEFAULT 0,
			status_2xx BIGINT DEFAULT 0,
			status_3xx BIGINT DEFAULT 0,
			status_4xx BIGINT DEFAULT 0,
			status_5xx BIGINT DEFAULT 0,
			avg_latency_ms DOUBLE PRECISION DEFAULT 0,
			latency_p50_ms DOUBLE PRECISION DEFAULT 0,
			latency_p95_ms DOUBLE PRECISION DEFAULT 0,
			latency_p99_ms DOUBLE PRECISION DEFAULT 0,
			avg_upstream_ms DOUBLE PRECISION DEFAULT 0,
			avg_request_size DOUBLE PRECISION DEFAULT 0,
			avg_response_size DOUBLE PRECISION DEFAULT 0,
			cache_hits BIGINT DEFAULT 0,
			cache_misses BIGINT DEFAULT 0,
			unique_ips INT DEFAULT 0,
			connection_count BIGINT DEFAULT 0
		);

		CREATE INDEX IF NOT EXISTS idx_metrics_agent_domain ON metrics (agent_id, domain, time DESC);

		CREATE TABLE IF NOT EXISTS visitor_ips (
			time TIMESTAMPTZ NOT NULL,
			agent_id VARCHAR(255) NOT NULL,
			domain VARCHAR(255) NOT NULL,
			ip_address VARCHAR(45) NOT NULL,
			request_count BIGINT DEFAULT 0
		);
		CREATE INDEX IF NOT EXISTS idx_visitor_ips_time_agent ON visitor_ips (agent_id, time DESC);
		CREATE INDEX IF NOT EXISTS idx_visitor_ips_ip ON visitor_ips (ip_address);
		CREATE INDEX IF NOT EXISTS idx_visitor_ips_agent_domain_time ON visitor_ips (agent_id, domain, time DESC);

		CREATE TABLE IF NOT EXISTS geo_cache (
			ip_address VARCHAR(45) PRIMARY KEY,
			country VARCHAR(100) DEFAULT '',
			country_code VARCHAR(10) DEFAULT '',
			city VARCHAR(100) DEFAULT '',
			region VARCHAR(100) DEFAULT '',
			lat DOUBLE PRECISION DEFAULT 0,
			lon DOUBLE PRECISION DEFAULT 0,
			isp VARCHAR(255) DEFAULT '',
			looked_up_at TIMESTAMPTZ DEFAULT NOW()
		);

		-- Alert rules
		CREATE TABLE IF NOT EXISTS alert_rules (
			id SERIAL PRIMARY KEY,
			user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			alert_type VARCHAR(50) NOT NULL,
			name VARCHAR(255) NOT NULL,
			config JSONB NOT NULL DEFAULT '{}',
			enabled BOOLEAN DEFAULT true,
			cooldown_minutes INTEGER DEFAULT 5,
			last_triggered_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_alert_rules_user_id ON alert_rules(user_id);
		CREATE INDEX IF NOT EXISTS idx_alert_rules_type ON alert_rules(alert_type);

		-- Notification channels
		CREATE TABLE IF NOT EXISTS notification_channels (
			id SERIAL PRIMARY KEY,
			user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			name VARCHAR(255) NOT NULL,
			channel_type VARCHAR(50) NOT NULL,
			config JSONB NOT NULL DEFAULT '{}',
			enabled BOOLEAN DEFAULT true,
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_notification_channels_user_id ON notification_channels(user_id);

		-- Alert rule / channel link
		CREATE TABLE IF NOT EXISTS alert_rule_channels (
			rule_id INTEGER NOT NULL REFERENCES alert_rules(id) ON DELETE CASCADE,
			channel_id INTEGER NOT NULL REFERENCES notification_channels(id) ON DELETE CASCADE,
			PRIMARY KEY (rule_id, channel_id)
		);

		-- Alert history
		CREATE TABLE IF NOT EXISTS alert_history (
			id BIGSERIAL PRIMARY KEY,
			user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			rule_id INTEGER REFERENCES alert_rules(id) ON DELETE SET NULL,
			alert_type VARCHAR(50) NOT NULL,
			severity VARCHAR(20) NOT NULL DEFAULT 'warning',
			title VARCHAR(255) NOT NULL,
			message TEXT NOT NULL,
			metadata JSONB DEFAULT '{}',
			resolved BOOLEAN DEFAULT false,
			resolved_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_alert_history_user_id ON alert_history(user_id, created_at DESC);
		CREATE INDEX IF NOT EXISTS idx_alert_history_rule_id ON alert_history(rule_id);

		-- Role-based auth (role column created in table definition above)
		ALTER TABLE users ADD COLUMN IF NOT EXISTS role VARCHAR(50) DEFAULT 'member';

		-- Resource assignment tables
		CREATE TABLE IF NOT EXISTS user_dns_providers (
			user_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			dns_provider_id INT NOT NULL REFERENCES dns_providers(id) ON DELETE CASCADE,
			PRIMARY KEY (user_id, dns_provider_id)
		);

		CREATE TABLE IF NOT EXISTS user_agents (
			user_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			agent_id INT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
			PRIMARY KEY (user_id, agent_id)
		);

		-- System settings
		CREATE TABLE IF NOT EXISTS system_settings (
			key VARCHAR(100) PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at TIMESTAMPTZ DEFAULT NOW()
		);
		INSERT INTO system_settings (key, value) VALUES ('registration_mode', 'open') ON CONFLICT DO NOTHING;

		-- Password change tracking (for JWT invalidation)
		ALTER TABLE users ADD COLUMN IF NOT EXISTS password_changed_at TIMESTAMPTZ;

		-- Account suspension
		ALTER TABLE users ADD COLUMN IF NOT EXISTS suspended BOOLEAN DEFAULT false;
		ALTER TABLE users ADD COLUMN IF NOT EXISTS suspended_at TIMESTAMPTZ;
		ALTER TABLE users ADD COLUMN IF NOT EXISTS suspended_reason TEXT DEFAULT '';

		-- Password reset tokens
		CREATE TABLE IF NOT EXISTS password_reset_tokens (
			id BIGSERIAL PRIMARY KEY,
			user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			token VARCHAR(128) NOT NULL UNIQUE,
			expires_at TIMESTAMPTZ NOT NULL,
			used_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_password_reset_tokens_token ON password_reset_tokens(token);
		CREATE INDEX IF NOT EXISTS idx_password_reset_tokens_user_id ON password_reset_tokens(user_id);

		-- User API keys
		CREATE TABLE IF NOT EXISTS user_api_keys (
			id BIGSERIAL PRIMARY KEY,
			user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			name VARCHAR(255) NOT NULL,
			key_hash VARCHAR(64) NOT NULL UNIQUE,
			key_prefix VARCHAR(12) NOT NULL,
			last_used_at TIMESTAMPTZ,
			expires_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_user_api_keys_hash ON user_api_keys(key_hash);
		CREATE INDEX IF NOT EXISTS idx_user_api_keys_user_id ON user_api_keys(user_id);
	`

	_, err := tx.Exec(ctx, query)
	if err != nil {
		return fmt.Errorf("initial schema DDL failed: %w", err)
	}

	// TimescaleDB hypertable creation must happen outside the DDL transaction
	// for some drivers, but pgx handles it fine within the same transaction.
	// However, create_hypertable and the visitor_ips migration are safe here
	// because they use if_not_exists.

	if _, err := tx.Exec(ctx,
		"SELECT create_hypertable('metrics', 'time', if_not_exists => TRUE)"); err != nil {
		return fmt.Errorf("metrics hypertable creation failed: %w", err)
	}

	// Migrate visitor_ips to hypertable (drop PK + id column if they exist)
	migrateVisitorIPs := `
		ALTER TABLE visitor_ips DROP CONSTRAINT IF EXISTS visitor_ips_pkey;
		ALTER TABLE visitor_ips DROP COLUMN IF EXISTS id;
		ALTER TABLE visitor_ips DROP COLUMN IF EXISTS created_at;
		SELECT create_hypertable('visitor_ips', 'time', migrate_data => TRUE, if_not_exists => TRUE);
	`
	if _, err := tx.Exec(ctx, migrateVisitorIPs); err != nil {
		return fmt.Errorf("visitor_ips hypertable migration failed: %w", err)
	}

	return nil
}
