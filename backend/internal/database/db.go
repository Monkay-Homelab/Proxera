package database

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
)

var DB *pgxpool.Pool

// Connect establishes connection to PostgreSQL
func Connect() error {
	sslMode := os.Getenv("DB_SSL_MODE")
	if sslMode == "" {
		sslMode = "prefer"
	}

	dsn := fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=%s",
		os.Getenv("DB_USER"),
		os.Getenv("DB_PASSWORD"),
		os.Getenv("DB_HOST"),
		os.Getenv("DB_PORT"),
		os.Getenv("DB_NAME"),
		sslMode,
	)

	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return fmt.Errorf("unable to parse database config: %w", err)
	}

	config.MaxConns = 25
	config.MinConns = 5

	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		return fmt.Errorf("unable to create connection pool: %w", err)
	}

	// Test the connection
	err = pool.Ping(context.Background())
	if err != nil {
		return fmt.Errorf("unable to ping database: %w", err)
	}

	DB = pool
	slog.Info("Database connected successfully", "component", "db", "max_conns", config.MaxConns, "min_conns", config.MinConns)
	return nil
}

// Initialize runs schema migrations and sets up continuous aggregates,
// refresh policies, retention policies, and background cleanup tasks.
func Initialize() error {
	// Run versioned schema migrations
	if err := RunMigrations(); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	// Create continuous aggregates (must be separate Exec calls - can't be in same transaction as other DDL)
	agg15min := `
		CREATE MATERIALIZED VIEW IF NOT EXISTS metrics_15min
		WITH (timescaledb.continuous) AS
		SELECT
			time_bucket('15 minutes', time)              AS bucket,
			agent_id,
			domain,
			SUM(request_count)                           AS request_count,
			SUM(bytes_sent)                              AS bytes_sent,
			SUM(bytes_received)                          AS bytes_received,
			SUM(status_2xx)                              AS status_2xx,
			SUM(status_3xx)                              AS status_3xx,
			SUM(status_4xx)                              AS status_4xx,
			SUM(status_5xx)                              AS status_5xx,
			SUM(avg_latency_ms * request_count)          AS latency_weight,
			SUM(latency_p50_ms * request_count)          AS p50_weight,
			SUM(latency_p95_ms * request_count)          AS p95_weight,
			SUM(latency_p99_ms * request_count)          AS p99_weight,
			SUM(avg_upstream_ms * request_count)         AS upstream_weight,
			SUM(avg_request_size * request_count)        AS req_size_weight,
			SUM(avg_response_size * request_count)       AS res_size_weight,
			SUM(cache_hits)                              AS cache_hits,
			SUM(cache_misses)                            AS cache_misses,
			SUM(unique_ips)                              AS unique_ips,
			SUM(connection_count)                        AS connection_count
		FROM metrics
		GROUP BY bucket, agent_id, domain;
	`
	if _, err := DB.Exec(context.Background(), agg15min); err != nil {
		slog.Warn("metrics_15min aggregate creation failed", "component", "db", "error", err)
	}

	aggHourly := `
		CREATE MATERIALIZED VIEW IF NOT EXISTS metrics_hourly
		WITH (timescaledb.continuous) AS
		SELECT
			time_bucket('1 hour', time)                  AS bucket,
			agent_id,
			domain,
			SUM(request_count)                           AS request_count,
			SUM(bytes_sent)                              AS bytes_sent,
			SUM(bytes_received)                          AS bytes_received,
			SUM(status_2xx)                              AS status_2xx,
			SUM(status_3xx)                              AS status_3xx,
			SUM(status_4xx)                              AS status_4xx,
			SUM(status_5xx)                              AS status_5xx,
			SUM(avg_latency_ms * request_count)          AS latency_weight,
			SUM(latency_p50_ms * request_count)          AS p50_weight,
			SUM(latency_p95_ms * request_count)          AS p95_weight,
			SUM(latency_p99_ms * request_count)          AS p99_weight,
			SUM(avg_upstream_ms * request_count)         AS upstream_weight,
			SUM(avg_request_size * request_count)        AS req_size_weight,
			SUM(avg_response_size * request_count)       AS res_size_weight,
			SUM(cache_hits)                              AS cache_hits,
			SUM(cache_misses)                            AS cache_misses,
			SUM(unique_ips)                              AS unique_ips,
			SUM(connection_count)                        AS connection_count
		FROM metrics
		GROUP BY bucket, agent_id, domain;
	`
	if _, err := DB.Exec(context.Background(), aggHourly); err != nil {
		slog.Warn("metrics_hourly aggregate creation failed", "component", "db", "error", err)
	}

	// Add refresh policies for continuous aggregates
	refreshPolicies := `
		SELECT add_continuous_aggregate_policy('metrics_15min',
			start_offset  => INTERVAL '1 hour',
			end_offset    => INTERVAL '5 minutes',
			schedule_interval => INTERVAL '15 minutes',
			if_not_exists => TRUE
		);
		SELECT add_continuous_aggregate_policy('metrics_hourly',
			start_offset  => INTERVAL '3 hours',
			end_offset    => INTERVAL '30 minutes',
			schedule_interval => INTERVAL '1 hour',
			if_not_exists => TRUE
		);
	`
	if _, err := DB.Exec(context.Background(), refreshPolicies); err != nil {
		slog.Warn("Refresh policy creation failed", "component", "db", "error", err)
	}

	slog.Info("Database tables initialized", "component", "db")

	// Background: cleanup geo_cache, backfill aggregates, then add retention policies
	go func() {
		ctx := context.Background()

		// Cleanup geo_cache (regular table, no retention policy available)
		if _, err := DB.Exec(ctx,
			`DELETE FROM geo_cache WHERE looked_up_at < NOW() - INTERVAL '90 days'`); err != nil {
			slog.Warn("geo_cache cleanup failed", "component", "db", "error", err)
		}

		slog.Info("Backfilling continuous aggregates...", "component", "db")

		if _, err := DB.Exec(ctx,
			"CALL refresh_continuous_aggregate('metrics_15min', NULL, NOW())"); err != nil {
			slog.Warn("metrics_15min backfill failed", "component", "db", "error", err)
			return // Don't add retention if backfill failed
		}

		if _, err := DB.Exec(ctx,
			"CALL refresh_continuous_aggregate('metrics_hourly', NULL, NOW())"); err != nil {
			slog.Warn("metrics_hourly backfill failed", "component", "db", "error", err)
			return
		}

		slog.Info("Backfill complete, adding retention policies", "component", "db")

		if _, err := DB.Exec(ctx,
			"SELECT add_retention_policy('metrics', INTERVAL '30 days', if_not_exists => TRUE)"); err != nil {
			slog.Warn("metrics retention policy failed", "component", "db", "error", err)
		}
		if _, err := DB.Exec(ctx,
			"SELECT add_retention_policy('metrics_15min', INTERVAL '365 days', if_not_exists => TRUE)"); err != nil {
			slog.Warn("metrics_15min retention policy failed", "component", "db", "error", err)
		}
		if _, err := DB.Exec(ctx,
			"SELECT add_retention_policy('metrics_hourly', INTERVAL '365 days', if_not_exists => TRUE)"); err != nil {
			slog.Warn("metrics_hourly retention policy failed", "component", "db", "error", err)
		}
		if _, err := DB.Exec(ctx,
			"SELECT add_retention_policy('visitor_ips', INTERVAL '90 days', if_not_exists => TRUE)"); err != nil {
			slog.Warn("visitor_ips retention policy failed", "component", "db", "error", err)
		}

		slog.Info("Retention policies active", "component", "db")
	}()

	return nil
}
