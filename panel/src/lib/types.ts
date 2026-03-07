/* Proxera Panel — shared TypeScript interfaces */

export interface Agent {
	id: number;
	agent_id: string;
	name: string;
	api_key?: string;
	status: string;
	version: string;
	os: string;
	arch: string;
	last_seen: string;
	ip_address: string;
	lan_ip: string;
	wan_ip: string;
	host_count: number;
	dns_record_count: number;
	nginx_version: string;
	crowdsec_installed: boolean;
	metrics_interval: number;
	created_at: string;
	updated_at: string;
}

export interface HostConfig {
	id: number;
	domain: string;
	upstream_url: string;
	ssl: boolean;
	websocket: boolean;
	provider_id: number;
	agent_id: string;
	config: AdvancedConfig;
	created_at: string;
	updated_at: string;
}

export interface AdvancedConfig {
	headers?: Record<string, string>;
	hide_headers?: string[];
	disabled_proxy_headers?: string[];
	disabled_hide_headers?: string[];
	basic_auth?: { username: string; password: string };
	rate_limit?: { requests_per_second: number; burst: number; zone_size: string };
	ip_allowlist?: string[];
	ip_blocklist?: string[];
	hsts?: { enabled: boolean; max_age: number; include_subdomains: boolean; preload: boolean };
	cors?: {
		enabled: boolean;
		origins: string[];
		methods: string[];
		headers: string[];
		credentials: boolean;
		max_age: number;
	};
	security_headers?: {
		csp: string;
		referrer_policy: string;
		permissions_policy: string;
		x_frame_options: string;
	};
	timeouts?: { connect: string; send: string; read: string };
	client_max_body_size?: string;
	gzip?: { enabled: boolean; types: string[]; min_length: number };
	proxy_buffering?: { enabled: boolean; buffers: string; buffer_size: string };
	proxy_request_buffering?: boolean;
	proxy_ssl?: { verify: boolean; server_name: string };
	redirects?: { source: string; target: string; code: number }[];
	custom_error_pages?: Record<string, string>;
	locations?: { path: string; upstream_url: string; websocket: boolean; headers?: Record<string, string> }[];
	load_balancing?: {
		method: string;
		servers: { address: string; weight: number }[];
	};
	custom_nginx_config?: string;
	http2?: boolean;
	access_log?: boolean;
	server_aliases?: string[];
	redirect_only?: boolean;
}

export interface DnsProvider {
	id: number;
	zone_id: string;
	domain: string;
	provider: string;
	created_at: string;
}

export interface DnsRecord {
	id: string;
	type: string;
	name: string;
	content: string;
	ttl: number;
	proxied: boolean;
	priority?: number;
	agent_id: string;
	created_at: string;
	modified_on: string;
}

export interface Certificate {
	id: number;
	domain: string;
	san_domains: string[];
	status: string;
	expires_at: string;
	created_at: string;
	certificate_pem?: string;
	private_key_pem?: string;
	issuer_pem?: string;
	provider_id: number;
}

export interface MetricsBucket {
	bucket: string;
	request_count: number;
	bytes_sent: number;
	bytes_received: number;
	status_2xx: number;
	status_3xx: number;
	status_4xx: number;
	status_5xx: number;
	latency_p50_ms: number;
	latency_p95_ms: number;
	latency_p99_ms: number;
	avg_upstream_ms: number;
	avg_request_size: number;
	avg_response_size: number;
	cache_hits: number;
	cache_misses: number;
	unique_ips: number;
	connection_count: number;
}

export interface MetricsSummary {
	total_requests: number;
	total_bytes_sent: number;
	total_bytes_received: number;
	avg_latency_ms: number;
	error_rate: number;
	unique_ips: number;
	total_connections: number;
	cache_hit_rate: number;
}

export interface MetricsData {
	summary: MetricsSummary;
	buckets: MetricsBucket[];
	domains: string[];
	agents: { agent_id: string; name: string }[];
}

export interface Visitor {
	ip: string;
	country: string;
	country_code: string;
	city: string;
	request_count: number;
	bytes_sent: number;
	last_seen: string;
}

export interface BlockedConnection {
	ip: string;
	country: string;
	city: string;
	as_number: string;
	as_org: string;
	reason: string;
	agent_name: string;
	blocked_at: string;
}

export interface CrowdSecDecision {
	id: number;
	origin: string;
	type: string;
	scope: string;
	value: string;
	duration: string;
	scenario: string;
	created_at: string;
}

export interface CrowdSecAlert {
	id: number;
	created_at: string;
	scenario: string;
	message: string;
	source: { ip: string; range: string; scope: string };
	decisions: CrowdSecDecision[];
}

export interface CrowdSecCollection {
	name: string;
	status: string;
	description: string;
}

export interface CrowdSecBouncer {
	name: string;
	type: string;
	last_pull: string;
}

export interface User {
	id: number;
	name: string;
	email: string;
	role: string;
	created_at: string;
}

export interface AlertRule {
	id: number;
	user_id: number;
	alert_type: string;
	name: string;
	config: Record<string, any>;
	enabled: boolean;
	cooldown_minutes: number;
	last_triggered_at: string | null;
	channel_ids: number[];
	created_at: string;
	updated_at: string;
}

export interface NotificationChannel {
	id: number;
	user_id: number;
	name: string;
	channel_type: string;
	config: Record<string, any>;
	enabled: boolean;
	created_at: string;
	updated_at: string;
}

export interface AlertHistoryEntry {
	id: number;
	user_id: number;
	rule_id: number | null;
	alert_type: string;
	severity: string;
	title: string;
	message: string;
	metadata: Record<string, any>;
	resolved: boolean;
	resolved_at: string | null;
	created_at: string;
}
