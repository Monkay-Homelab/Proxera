<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api';
	import { navRefresh } from '$lib/navRefresh';
	import type { Agent, DnsProvider } from '$lib/types';

	interface BasicAuth { username: string; password: string }
	interface RateLimit { requests_per_second: number; burst: number; zone_size?: string }
	interface HstsConfig { enabled: boolean; max_age: number; include_subdomains: boolean; preload: boolean }
	interface CorsConfig { enabled: boolean; dynamic?: boolean; allowed_origins: string[]; allowed_methods: string[]; allowed_headers: string[]; allow_credentials: boolean; max_age: number }
	interface SecurityHeaders { csp: string; referrer_policy: string; permissions_policy: string; x_frame_options: string }
	interface TimeoutsConfig { connect: number | string; send: number | string; read: number | string }
	interface GzipConfig { enabled: boolean; types: string[]; min_length: number; comp_level: number }
	interface ProxyBuffering { enabled: boolean; buffer_size: string; buffers_count: number; buffers_size: string }
	interface ProxySsl { verify: boolean; server_name: boolean | string }
	interface RedirectEntry { source: string; target: string; code: number }
	interface LocationEntry { path: string; upstream_url: string; websocket: boolean; headers?: Record<string, string>; use_location_proxy_headers?: boolean; disabled_proxy_headers?: string[] }
	interface LbServer { address: string; weight: number }
	interface LoadBalancing { method: string; servers: LbServer[] }
	interface ListItem { value: string }
	interface LocationListItem extends LocationEntry { _expanded: boolean; _headerPairs: { key: string; value: string }[]; _disabledProxyHeaders: string[]; exact?: boolean; strip_prefix?: boolean }
	interface ErrorPageItem { code: string; path: string }
	interface CertRecord { id: number; domain: string; san?: string; status: string; issued_at?: string; expires_at?: string; certificate_pem?: string; private_key_pem?: string; issuer_pem?: string }
	interface BackupEntry { filename: string; size: number; size_bytes?: number; created_at: string; timestamp?: string }

	interface HostEditConfig {
		headers: Record<string, string>;
		hide_headers: string[];
		disabled_proxy_headers: string[];
		disabled_hide_headers: string[];
		basic_auth: BasicAuth | null;
		rate_limit: RateLimit | null;
		ip_allowlist: string[];
		ip_blocklist: string[];
		trusted_proxies: string[];
		cloudflare_real_ip: boolean;
		http_proxy: boolean;
		hsts: HstsConfig | null;
		cors: CorsConfig | null;
		security_headers: SecurityHeaders | null;
		timeouts: TimeoutsConfig | null;
		client_max_body_size: string;
		gzip: GzipConfig | null;
		proxy_buffering: ProxyBuffering | null;
		proxy_request_buffering: boolean | null;
		proxy_ssl: ProxySsl | null;
		redirects: RedirectEntry[];
		custom_error_pages: Record<string, string>;
		locations: LocationEntry[];
		load_balancing: LoadBalancing | null;
		custom_nginx_config: string;
		http2: boolean;
		access_log: boolean;
		server_aliases: string[];
		redirect_only: boolean;
	}

	const providerId = $page.params.id;
	const hostId = $page.params.hostId;
	const isNew = hostId === 'new';

	let parentDomain = '';
	let loading = true;
	let saving = false;
	let error = '';
	let successMsg = '';

	// Basic config
	let formDomain = '';
	let formUpstream = '';
	let formSSL = false;
	let formWebSocket = false;
	let formCertificateId: number | null = null;
	let formAgentId: number | string | null = null;

	// Certificate state
	let certificates: CertRecord[] = [];
	let certsLoaded = false;
	let matchedCert: CertRecord | null = null;

	// Agent state
	let agents: Agent[] = [];
	let agentsLoaded = false;

	// Advanced config
	let config: HostEditConfig = {
		headers: {},
		hide_headers: [],
		disabled_proxy_headers: [],
		disabled_hide_headers: [],
		basic_auth: null,
		rate_limit: null,
		ip_allowlist: [],
		ip_blocklist: [],
		trusted_proxies: [],
		cloudflare_real_ip: false,
		http_proxy: false,
		hsts: null,
		cors: null,
		security_headers: null,
		timeouts: null,
		client_max_body_size: '',
		gzip: null,
		proxy_buffering: null,
		proxy_request_buffering: null,
		proxy_ssl: null,
		redirects: [],
		custom_error_pages: {},
		locations: [],
		load_balancing: null,
		custom_nginx_config: '',
		http2: true,
		access_log: true,
		server_aliases: [],
		redirect_only: false
	};

	// Active section for nav
	let activeSection = 'basic';

	// Backup history state
	let backups: BackupEntry[] = [];
	let backupsLoading = false;
	let backupsLoaded = false;
	let restoring: string | null = null; // filename being restored
	let restoreMsg = '';
	let restoreError = '';

	// Backup viewer modal
	let viewModalOpen = false;
	let viewModalFilename = '';
	let viewModalContent = '';
	let viewModalLoading = false;
	let viewModalError = '';

	// UI lists for dynamic items
	let allowlistList: ListItem[] = [];
	let blocklistList: ListItem[] = [];
	let trustedProxiesList: ListItem[] = [];
	let redirectsList: RedirectEntry[] = [];
	let errorPagesList: ErrorPageItem[] = [];
	let locationsList: LocationListItem[] = [];
	let lbServersList: LbServer[] = [];
	let serverAliasesList: ListItem[] = [];
	let corsOriginsList: ListItem[] = [];

	// Preset options
	const HTTP_METHODS = ['GET', 'POST', 'PUT', 'DELETE', 'PATCH', 'OPTIONS', 'HEAD'];
	const CORS_HEADERS = ['Authorization', 'Content-Type', 'X-Requested-With', 'Accept', 'Origin', 'X-CSRF-Token'];
	const GZIP_TYPES = [
		{ label: 'text/plain', value: 'text/plain' },
		{ label: 'text/css', value: 'text/css' },
		{ label: 'application/json', value: 'application/json' },
		{ label: 'application/javascript', value: 'application/javascript' },
		{ label: 'text/xml', value: 'text/xml' },
		{ label: 'application/xml', value: 'application/xml' },
		{ label: 'image/svg+xml', value: 'image/svg+xml' },
		{ label: 'application/wasm', value: 'application/wasm' },
	];
		const DEFAULT_PROXY_HEADERS = [
		{ key: 'Host', value: '$host', desc: 'Forward the original Host header. Required when upstream routing depends on the domain name.' },
		{ key: 'X-Real-IP', value: '$remote_addr', desc: "Pass the visitor's real IP address to your upstream server." },
		{ key: 'X-Forwarded-For', value: '$proxy_add_x_forwarded_for', desc: 'Full chain of client and proxy IPs the request passed through.' },
		{ key: 'X-Forwarded-Proto', value: '$scheme', desc: 'Tell your upstream whether the original request was HTTP or HTTPS.' },
		{ key: 'X-Forwarded-Host', value: '$host', desc: 'Pass the original domain name the visitor used.' },
		{ key: 'X-Forwarded-Port', value: '$server_port', desc: 'Pass the original port number of the incoming request.' },
	];
	const EXTRA_PROXY_HEADERS = [
		{ key: 'X-Request-ID', value: '$request_id', desc: 'Attach a unique ID to each request for tracing and debugging across services.' },
		{ key: 'X-Original-URI', value: '$request_uri', desc: 'Pass the original request path and query string before any nginx rewrites.' },
	];
	const DEFAULT_HIDE_HEADERS = [
		{ key: 'X-Powered-By', desc: 'Hides backend technology info like "Express", "PHP/8.1", or "ASP.NET".' },
		{ key: 'Server', desc: 'Hides the web server software name like "Apache" or "openresty".' },
		{ key: 'X-AspNet-Version', desc: 'Hides the ASP.NET runtime version number.' },
		{ key: 'X-AspNetMvc-Version', desc: 'Hides the ASP.NET MVC framework version.' },
	];
	const EXTRA_HIDE_HEADERS = [
		{ key: 'X-Runtime', desc: 'Hides request processing time exposed by frameworks like Ruby on Rails.' },
		{ key: 'X-Request-Id', desc: "Hides the upstream's internal request tracking identifier." },
	];
	const PERMISSIONS = ['camera', 'microphone', 'geolocation', 'payment', 'usb', 'fullscreen', 'autoplay'];
	const HSTS_PRESETS = [
		{ label: '1 year', value: 31536000 },
		{ label: '6 months', value: 15768000 },
		{ label: '1 month', value: 2592000 },
		{ label: '1 week', value: 604800 },
	];
	const TIMEOUT_PRESETS = [
		{ label: '30s', value: 30 },
		{ label: '60s', value: 60 },
		{ label: '120s', value: 120 },
		{ label: '300s', value: 300 },
		{ label: '600s', value: 600 },
	];
	const BODY_SIZE_PRESETS = ['', '1m', '10m', '50m', '100m', '500m', '1g', '2g', '5g', '10g'];
	const CORS_MAX_AGE_PRESETS = [
		{ label: 'None', value: 0 },
		{ label: '1 hour', value: 3600 },
		{ label: '12 hours', value: 43200 },
		{ label: '1 day', value: 86400 },
		{ label: '1 week', value: 604800 },
	];
	const RATE_PRESETS = [1, 5, 10, 25, 50, 100];
	const BURST_PRESETS = [5, 10, 20, 50, 100, 200];
	const BUFFER_SIZE_PRESETS = ['4k', '8k', '16k', '32k'];
	const BUFFER_COUNT_PRESETS = [4, 8, 16, 32];
	const GZIP_LEVEL_PRESETS = [
		{ label: 'Default', value: 0 },
		{ label: '1 (Fastest)', value: 1 },
		{ label: '4 (Balanced)', value: 4 },
		{ label: '6 (Good)', value: 6 },
		{ label: '9 (Best)', value: 9 },
	];
	const GZIP_MINLEN_PRESETS = [
		{ label: 'Default', value: 0 },
		{ label: '256 B', value: 256 },
		{ label: '1 KB', value: 1024 },
		{ label: '2 KB', value: 2048 },
		{ label: '4 KB', value: 4096 },
	];
	const ERROR_CODES = ['400', '401', '403', '404', '500', '502', '503', '504'];
	const REDIRECT_CODES = [
		{ label: '301 Permanent', value: 301 },
		{ label: '302 Temporary', value: 302 },
		{ label: '307 Temp (keep method)', value: 307 },
		{ label: '308 Perm (keep method)', value: 308 },
	];
	const LB_WEIGHT_PRESETS = [1, 2, 3, 5, 10];

	// Chip toggle helper
	function toggleChip(arr: string[], item: string) {
		const idx = arr.indexOf(item);
		if (idx >= 0) {
			return arr.filter((_: string, i: number) => i !== idx);
		}
		return [...arr, item];
	}

	// Count configured items in a section
	function sectionBadge(name: string) {
		let count = 0;
		if (name === 'headers') {
			count += 6 - config.disabled_proxy_headers.length;
			count += Object.keys(config.headers).length;
			count += 4 - config.disabled_hide_headers.length;
			count += config.hide_headers.length;
			if (config.basic_auth) count++;
		} else if (name === 'security') {
			if (config.rate_limit) count++;
			count += allowlistList.length;
			count += blocklistList.length;
			count += trustedProxiesList.length;
			if (config.cloudflare_real_ip) count++;
			if (config.http_proxy) count++;
			if (config.hsts) count++;
			if (config.cors) count++;
			if (config.security_headers) count++;
		} else if (name === 'performance') {
			if (config.timeouts) count++;
			if (config.client_max_body_size) count++;
			if (config.gzip) count++;
			if (config.proxy_buffering) count++;
			if (config.proxy_request_buffering === false) count++;
			if (config.proxy_ssl) count++;
			if (!config.access_log) count++;
		} else if (name === 'routing') {
			count += redirectsList.length;
			count += errorPagesList.length;
		} else if (name === 'locations') {
			count += locationsList.filter(l => l.path && l.upstream_url).length;
		} else if (name === 'loadbalancing') {
			if (config.load_balancing) count += lbServersList.length;
		} else if (name === 'advanced') {
			if (config.custom_nginx_config.trim()) count++;
		}
		return count;
	}

	onMount(async () => {
		try {
			const res = await api('/api/dns/providers');
			if (res.ok) {
				const providers = await res.json();
				const match = providers.find((p: DnsProvider) => p.id === parseInt(providerId ?? ''));
				if (match) parentDomain = match.domain;
			}
		} catch (err) {
			console.error('Failed to fetch provider:', err);
		}

		await fetchAgents();

		if (!isNew) {
			await fetchHost();
		} else {
			// New hosts start as a basic proxy — no default headers pre-enabled
			config.disabled_proxy_headers = ['Host', 'X-Real-IP', 'X-Forwarded-For', 'X-Forwarded-Proto', 'X-Forwarded-Host', 'X-Forwarded-Port'];
			config.disabled_hide_headers = ['X-Powered-By', 'Server', 'X-AspNet-Version', 'X-AspNetMvc-Version'];
		}

		loading = false;
	});

	async function fetchAgents() {
		try {
			const res = await api('/api/agents');
			if (res.ok) agents = await res.json();
		} catch (err) {
			console.error('Failed to fetch agents:', err);
		}
		agentsLoaded = true;
	}

	async function fetchHost() {
		try {
			const res = await api(`/api/hosts/${providerId}/configs/${hostId}`);
			if (res.ok) {
				const data = await res.json();
				formDomain = extractSubdomain(data.domain);
				formUpstream = data.upstream_url;
				formSSL = data.ssl;
				formWebSocket = data.websocket;
				formCertificateId = data.certificate_id;
				formAgentId = data.agent_id;

				if (data.config) {
					const c = data.config;
					if (c.headers) { config.headers = c.headers; }
					if (c.hide_headers) config.hide_headers = c.hide_headers;
					if (c.disabled_proxy_headers) config.disabled_proxy_headers = c.disabled_proxy_headers;
					if (c.disabled_hide_headers) config.disabled_hide_headers = c.disabled_hide_headers;
					if (c.basic_auth) config.basic_auth = c.basic_auth;
					if (c.rate_limit) config.rate_limit = c.rate_limit;
					if (c.ip_allowlist) { config.ip_allowlist = c.ip_allowlist; allowlistList = c.ip_allowlist.map((ip: string) => ({value:ip})); }
					if (c.ip_blocklist) { config.ip_blocklist = c.ip_blocklist; blocklistList = c.ip_blocklist.map((ip: string) => ({value:ip})); }
					if (c.trusted_proxies) { config.trusted_proxies = c.trusted_proxies; trustedProxiesList = c.trusted_proxies.map((ip: string) => ({value:ip})); }
					if (c.cloudflare_real_ip) config.cloudflare_real_ip = c.cloudflare_real_ip;
					if (c.http_proxy) config.http_proxy = c.http_proxy;
					if (c.hsts) config.hsts = c.hsts;
					if (c.cors) { config.cors = c.cors; corsOriginsList = (c.cors.allowed_origins || []).map((o: string) => ({value:o})); }
					if (c.security_headers) config.security_headers = c.security_headers;
					if (c.timeouts) config.timeouts = c.timeouts;
					if (c.client_max_body_size) config.client_max_body_size = c.client_max_body_size;
					if (c.gzip) config.gzip = c.gzip;
					if (c.proxy_buffering) config.proxy_buffering = c.proxy_buffering;
					if (c.proxy_request_buffering !== undefined && c.proxy_request_buffering !== null) config.proxy_request_buffering = c.proxy_request_buffering;
					if (c.proxy_ssl) config.proxy_ssl = c.proxy_ssl;
					if (c.redirects) { config.redirects = c.redirects; redirectsList = [...c.redirects]; }
					if (c.custom_error_pages) { config.custom_error_pages = c.custom_error_pages; errorPagesList = Object.entries(c.custom_error_pages).map(([k,v]) => ({code:k, path:v as string})); }
					if (c.locations) {
						const hostDisabled = c.disabled_proxy_headers || [];
						locationsList = c.locations.map((l: LocationEntry & { use_location_proxy_headers?: boolean; disabled_proxy_headers?: string[] }) => ({
							...l,
							_expanded: Object.keys(l.headers || {}).length > 0,
							_headerPairs: Object.entries(l.headers || {}).map(([key, value]) => ({ key, value })),
							_disabledProxyHeaders: l.use_location_proxy_headers ? (l.disabled_proxy_headers || []) : [...hostDisabled]
						}));
					}
					if (c.load_balancing) { config.load_balancing = c.load_balancing; lbServersList = [...c.load_balancing.servers]; }
					if (c.custom_nginx_config) config.custom_nginx_config = c.custom_nginx_config;
					if (c.http2 !== undefined && c.http2 !== null) config.http2 = c.http2;
					if (c.access_log !== undefined && c.access_log !== null) config.access_log = c.access_log;
					if (c.server_aliases) { config.server_aliases = c.server_aliases; serverAliasesList = c.server_aliases.map((a: string) => ({value:a})); }
					if (c.redirect_only) config.redirect_only = c.redirect_only;
				}

				if (formSSL) {
					await fetchCertificates();
					if (formCertificateId) {
						matchedCert = certificates.find(c => c.id === formCertificateId) || null;
					} else {
						autoMatchCert();
					}
				}
			} else {
				error = 'Failed to fetch host config';
			}
		} catch (err) {
			error = 'Failed to connect to API';
		}
	}

	function extractSubdomain(fullDomain: string) {
		if (!parentDomain) return fullDomain;
		if (fullDomain === parentDomain) return '@';
		if (fullDomain.endsWith('.' + parentDomain)) {
			return fullDomain.slice(0, -(parentDomain.length + 1));
		}
		return fullDomain;
	}

	function buildFullDomain(sub: string) {
		if (!sub || sub === '@') return parentDomain;
		return `${sub}.${parentDomain}`;
	}

	async function fetchCertificates() {
		if (certsLoaded) return;
		try {
			const res = await api('/api/certificates');
			if (res.ok) {
				certificates = (await res.json()).filter((c: CertRecord) => c.status === 'active' || c.status === 'expiring');
			}
		} catch (err) {
			console.error('Failed to fetch certificates:', err);
		}
		certsLoaded = true;
	}

	function findMatchingCert(domain: string) {
		if (!domain || certificates.length === 0) return null;
		let match = certificates.find(c => c.domain === domain);
		if (match) return match;
		match = certificates.find(c => c.san && c.san.split(',').some(s => s.trim() === domain));
		if (match) return match;
		const parts = domain.split('.');
		if (parts.length >= 2) {
			const wildcardDomain = '*.' + parts.slice(1).join('.');
			match = certificates.find(c =>
				c.domain === wildcardDomain ||
				(c.san && c.san.split(',').some(s => s.trim() === wildcardDomain))
			);
			if (match) return match;
		}
		return null;
	}

	function onSSLToggle() {
		formSSL = !formSSL;
		if (formSSL) {
			fetchCertificates().then(() => autoMatchCert());
		} else {
			formCertificateId = null;
			matchedCert = null;
		}
	}

	function autoMatchCert() {
		const fullDomain = buildFullDomain(formDomain.trim());
		matchedCert = findMatchingCert(fullDomain);
		formCertificateId = matchedCert ? matchedCert.id : null;
	}

	function buildPermissionsPolicy(perms: string[]) {
		return perms.map(p => `${p}=()`).join(', ');
	}

	function parsePermissionsPolicy(str: string) {
		if (!str) return [];
		return str.split(',').map(s => s.trim().replace(/=\(\)$/, '')).filter(Boolean);
	}

	function buildConfigPayload() {
		const cfg: Record<string, unknown> = {};

		if (Object.keys(config.headers).length > 0) {
			cfg.headers = { ...config.headers };
		}

		if (config.hide_headers.length > 0) cfg.hide_headers = config.hide_headers;
		if (config.disabled_proxy_headers.length > 0) cfg.disabled_proxy_headers = config.disabled_proxy_headers;
		if (config.disabled_hide_headers.length > 0) cfg.disabled_hide_headers = config.disabled_hide_headers;

		if (config.basic_auth && config.basic_auth.username) {
			cfg.basic_auth = config.basic_auth;
		}

		if (config.rate_limit && config.rate_limit.requests_per_second > 0) {
			cfg.rate_limit = config.rate_limit;
		}

		const ips_allow = allowlistList.map(i => i.value.trim()).filter(Boolean);
		if (ips_allow.length > 0) cfg.ip_allowlist = ips_allow;
		const ips_block = blocklistList.map(i => i.value.trim()).filter(Boolean);
		if (ips_block.length > 0) cfg.ip_blocklist = ips_block;
		const trusted = trustedProxiesList.map(i => i.value.trim()).filter(Boolean);
		if (trusted.length > 0) cfg.trusted_proxies = trusted;
		if (config.cloudflare_real_ip) cfg.cloudflare_real_ip = true;
		if (config.http_proxy) cfg.http_proxy = true;

		if (config.hsts && config.hsts.enabled) cfg.hsts = config.hsts;

		if (config.cors && config.cors.enabled) {
			const origins = corsOriginsList.map(i => i.value.trim()).filter(Boolean);
			const dynamic = config.cors.dynamic || (origins.length > 1 && !origins.includes('*'));
			cfg.cors = { ...config.cors, dynamic, allowed_origins: origins.length > 0 ? origins : ['*'] };
		}

		if (config.security_headers) {
			const sh = config.security_headers;
			if (sh.csp || sh.referrer_policy || sh.permissions_policy || sh.x_frame_options) {
				cfg.security_headers = sh;
			}
		}

		if (config.timeouts && (config.timeouts.connect || config.timeouts.send || config.timeouts.read)) {
			cfg.timeouts = config.timeouts;
		}

		if (config.client_max_body_size) cfg.client_max_body_size = config.client_max_body_size;

		if (config.gzip) cfg.gzip = config.gzip;

		if (config.proxy_buffering) cfg.proxy_buffering = config.proxy_buffering;

		if (config.proxy_request_buffering !== null && config.proxy_request_buffering !== undefined) {
			cfg.proxy_request_buffering = config.proxy_request_buffering;
		}

		if (config.proxy_ssl) cfg.proxy_ssl = config.proxy_ssl;

		const validRedirects = redirectsList.filter(r => r.source && r.target);
		if (validRedirects.length > 0) cfg.redirects = validRedirects.map(r => ({ source: r.source, target: r.target, code: r.code || 301 }));

		if (errorPagesList.length > 0) {
			const ep: Record<string, string> = {};
			errorPagesList.forEach((item: ErrorPageItem) => { if (item.code && item.path) ep[item.code] = item.path; });
			if (Object.keys(ep).length > 0) cfg.custom_error_pages = ep;
		}

		const validLocs = locationsList
			.filter((l: LocationListItem) => l.path && l.upstream_url)
			.map((item: LocationListItem) => {
				const headers: Record<string, string> = {};
				for (const { key, value } of (item._headerPairs || [])) {
					if (key && value) headers[key] = value;
				}
				const loc: Record<string, unknown> = { path: item.path, upstream_url: item.upstream_url, websocket: item.websocket };
				if (Object.keys(headers).length > 0) loc.headers = headers;
				if (item.exact) loc.exact = true;
				if (item.strip_prefix) loc.strip_prefix = true;
				loc.use_location_proxy_headers = true;
				if (item._disabledProxyHeaders.length > 0) loc.disabled_proxy_headers = item._disabledProxyHeaders;
				return loc;
			});
		if (validLocs.length > 0) cfg.locations = validLocs;

		if (lbServersList.length > 0 && config.load_balancing) {
			const validServers = lbServersList.filter(s => s.address);
			if (validServers.length > 0) {
				cfg.load_balancing = {
					method: config.load_balancing.method || 'round-robin',
					servers: validServers
				};
			}
		}

		if (config.custom_nginx_config.trim()) cfg.custom_nginx_config = config.custom_nginx_config;
		if (!config.http2) cfg.http2 = false;
		if (!config.access_log) cfg.access_log = false;

		const aliases = serverAliasesList.map(i => i.value.trim()).filter(Boolean);
		if (aliases.length > 0) cfg.server_aliases = aliases;
		if (config.redirect_only) cfg.redirect_only = true;

		return Object.keys(cfg).length > 0 ? cfg : null;
	}

	async function save() {
		if (!formUpstream.trim() && !config.redirect_only) return;
		saving = true;
		error = '';
		successMsg = '';

		const fullDomain = buildFullDomain(formDomain.trim());
		const body = {
			domain: fullDomain,
			upstream_url: formUpstream.trim(),
			ssl: formSSL,
			websocket: formWebSocket,
			certificate_id: formSSL && formCertificateId ? formCertificateId : null,
			agent_id: formAgentId || null,
			config: buildConfigPayload()
		};

		try {
			let url, method;
			if (isNew) {
				url = `/api/hosts/${providerId}/configs`;
				method = 'POST';
			} else {
				url = `/api/hosts/${providerId}/configs/${hostId}`;
				method = 'PATCH';
			}

			const res = await api(url, {
				method,
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify(body)
			});

			if (res.ok) {
				const data = await res.json();
				if (data.deploy_error) {
					error = `Host saved, but deploy failed: ${data.deploy_error}`;
				} else {
					successMsg = isNew ? 'Host created and deployed successfully' : 'Host updated and deployed successfully';
					if (isNew) {
						navRefresh.update(n => n + 1);
						setTimeout(() => goto(`/hosts/${providerId}/edit/${data.id}`), 500);
					}
				}
			} else {
				const data = await res.json();
				error = data.error || data.message || 'Failed to save host config';
			}
		} catch (err) {
			error = 'Failed to connect to API';
		} finally {
			saving = false;
		}
	}

	// List helpers
function addServerAlias() { serverAliasesList = [...serverAliasesList, { value: '' }]; }
	function removeServerAlias(i: number) { serverAliasesList = serverAliasesList.filter((_: ListItem, idx: number) => idx !== i); }
	function addAllowlistIP() { allowlistList = [...allowlistList, { value: '' }]; }
	function removeAllowlistIP(i: number) { allowlistList = allowlistList.filter((_: ListItem, idx: number) => idx !== i); }
	function addBlocklistIP() { blocklistList = [...blocklistList, { value: '' }]; }
	function removeBlocklistIP(i: number) { blocklistList = blocklistList.filter((_: ListItem, idx: number) => idx !== i); }
	function addTrustedProxy() { trustedProxiesList = [...trustedProxiesList, { value: '' }]; }
	function removeTrustedProxy(i: number) { trustedProxiesList = trustedProxiesList.filter((_: ListItem, idx: number) => idx !== i); }
	function addRedirect() { redirectsList = [...redirectsList, { source: '', target: '', code: 301 }]; }
	function removeRedirect(i: number) { redirectsList = redirectsList.filter((_: RedirectEntry, idx: number) => idx !== i); }
	function addErrorPage() { errorPagesList = [...errorPagesList, { code: '', path: '' }]; }
	function removeErrorPage(i: number) { errorPagesList = errorPagesList.filter((_: ErrorPageItem, idx: number) => idx !== i); }
	function addLocation() {
		locationsList = [...locationsList, {
			path: '', upstream_url: '', websocket: false,
			_expanded: false, _headerPairs: [],
			_disabledProxyHeaders: [...(config.disabled_proxy_headers || [])]
		} as LocationListItem];
	}
	function removeLocation(i: number) { locationsList = locationsList.filter((_: LocationListItem, idx: number) => idx !== i); }
	function moveLocation(i: number, dir: number) {
		const j = i + dir;
		if (j < 0 || j >= locationsList.length) return;
		const next = [...locationsList];
		[next[i], next[j]] = [next[j], next[i]];
		locationsList = next;
	}
	function addLBServer() { lbServersList = [...lbServersList, { address: '', weight: 1 }]; }
	function removeLBServer(i: number) { lbServersList = lbServersList.filter((_: LbServer, idx: number) => idx !== i); }
	function addCorsOrigin() { corsOriginsList = [...corsOriginsList, { value: '' }]; }
	function removeCorsOrigin(i: number) { corsOriginsList = corsOriginsList.filter((_: ListItem, idx: number) => idx !== i); }

	function toggleProxyHeader(hdr: { key: string; value: string }) {
		if (config.headers[hdr.key]) {
			const copy = { ...config.headers };
			delete copy[hdr.key];
			config.headers = copy;
		} else {
			config.headers = { ...config.headers, [hdr.key]: hdr.value };
		}
	}

	function toggleDefaultProxyHeader(key: string) {
		if (config.disabled_proxy_headers.includes(key)) {
			config.disabled_proxy_headers = config.disabled_proxy_headers.filter(h => h !== key);
		} else {
			config.disabled_proxy_headers = [...config.disabled_proxy_headers, key];
		}
	}

	function toggleDefaultHideHeader(key: string) {
		if (config.disabled_hide_headers.includes(key)) {
			config.disabled_hide_headers = config.disabled_hide_headers.filter(h => h !== key);
		} else {
			config.disabled_hide_headers = [...config.disabled_hide_headers, key];
		}
	}

	function toggleHideHeader(key: string) {
		if (config.hide_headers.includes(key)) {
			config.hide_headers = config.hide_headers.filter(h => h !== key);
		} else {
			config.hide_headers = [...config.hide_headers, key];
		}
	}

	function initBasicAuth() { if (!config.basic_auth) config.basic_auth = { username: '', password: '' }; }
	function clearBasicAuth() { config.basic_auth = null; }
	function initRateLimit() { if (!config.rate_limit) config.rate_limit = { requests_per_second: 10, burst: 20 }; }
	function clearRateLimit() { config.rate_limit = null; }
	function initHSTS() { if (!config.hsts) config.hsts = { enabled: true, max_age: 31536000, include_subdomains: true, preload: false }; }
	function initCORS() { if (!config.cors) { config.cors = { enabled: true, dynamic: false, allowed_origins: ['*'], allowed_methods: ['GET', 'POST', 'OPTIONS'], allowed_headers: ['*'], allow_credentials: false, max_age: 86400 }; corsOriginsList = [{value:'*'}]; } }
	function initSecurityHeaders() { if (!config.security_headers) config.security_headers = { csp: '', referrer_policy: 'strict-origin-when-cross-origin', permissions_policy: 'camera=(), microphone=(), geolocation=()', x_frame_options: 'SAMEORIGIN' }; }
	function initTimeouts() { if (!config.timeouts) config.timeouts = { connect: 60, send: 60, read: 60 }; }
	function initGzip() { if (!config.gzip) config.gzip = { enabled: true, types: ['text/plain', 'text/css', 'application/json', 'application/javascript'], min_length: 1024, comp_level: 0 }; }
	function initProxySSL() { if (!config.proxy_ssl) config.proxy_ssl = { verify: false, server_name: true }; }
	function initProxyBuffering() { if (!config.proxy_buffering) config.proxy_buffering = { enabled: true, buffer_size: '4k', buffers_count: 8, buffers_size: '8k' }; }
	function initLoadBalancing() { if (!config.load_balancing) config.load_balancing = { method: 'round-robin', servers: [] }; }

	async function loadBackups() {
		if (isNew || backupsLoading) return;
		backupsLoading = true;
		backupsLoaded = false;
		restoreMsg = '';
		restoreError = '';
		try {
			const res = await api(`/api/hosts/${providerId}/configs/${hostId}/backups`);
			if (res.ok) {
				const data = await res.json();
				backups = data.backups || [];
			} else {
				backups = [];
			}
		} catch (err) {
			backups = [];
		}
		backupsLoading = false;
		backupsLoaded = true;
	}

	async function restoreBackup(filename: string) {
		restoring = filename;
		restoreMsg = '';
		restoreError = '';
		try {
			const res = await api(`/api/hosts/${providerId}/configs/${hostId}/backups/restore`, {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ filename })
			});
			if (res.ok) {
				restoreMsg = `Restored ${filename} successfully.`;
				await loadBackups();
			} else {
				const data = await res.json().catch(() => ({}));
				restoreError = data.error || 'Restore failed';
			}
		} catch (err) {
			restoreError = 'Failed to connect to API';
		}
		restoring = null;
	}

	function formatBytes(bytes: number) {
		if (bytes < 1024) return `${bytes} B`;
		if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
		return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
	}

	function formatBackupDate(ts: string) {
		return new Date(ts).toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' });
	}

	async function openBackupViewer(b: BackupEntry) {
		viewModalFilename = b.filename;
		viewModalContent = '';
		viewModalError = '';
		viewModalLoading = true;
		viewModalOpen = true;
		try {
			const res = await api(`/api/hosts/${providerId}/configs/${hostId}/backups/${encodeURIComponent(b.filename)}`);
			if (res.ok) {
				const data = await res.json();
				viewModalContent = data.content || '';
			} else {
				const data = await res.json().catch(() => ({}));
				viewModalError = data.error || 'Failed to load backup';
			}
		} catch (err) {
			viewModalError = 'Failed to connect to API';
		}
		viewModalLoading = false;
	}

	function closeBackupViewer() {
		viewModalOpen = false;
	}
</script>

<svelte:head>
	<title>{isNew ? 'Add Host' : 'Edit Host'} - {parentDomain || 'Proxera'}</title>
</svelte:head>

<div class="page">
	{#if loading}
		<div class="placeholder"><div class="loader"></div><p>Loading...</p></div>
	{:else}
		<header class="page-head">
			<button class="breadcrumb" onclick={() => goto(`/hosts/${providerId}`)}>
				<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><line x1="19" y1="12" x2="5" y2="12"/><polyline points="12 19 5 12 12 5"/></svg>
				{parentDomain || 'Hosts'}
			</button>
			<h1>{isNew ? 'New Host' : 'Edit Host'}</h1>
			<p class="page-desc">Configure how traffic is routed from a domain to your backend server.</p>
		</header>

		{#if error}
			<div class="msg msg-error">
				<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><line x1="15" y1="9" x2="9" y2="15"/><line x1="9" y1="9" x2="15" y2="15"/></svg>
				{error}
			</div>
		{/if}
		{#if successMsg}
			<div class="msg msg-success">
				<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"/><polyline points="22 4 12 14.01 9 11.01"/></svg>
				{successMsg}
			</div>
		{/if}

		<div class="editor-layout">
			<nav class="editor-nav">
				<div class="nav-items">
					<button class="nav-item" class:active={activeSection === 'basic'} onclick={() => activeSection = 'basic'}>
						<span class="nav-label">Basic</span>
					</button>
					<button class="nav-item" class:active={activeSection === 'headers'} onclick={() => activeSection = 'headers'}>
						<span class="nav-label">Headers</span>
						{#if sectionBadge('headers') > 0}<span class="nav-badge">{sectionBadge('headers')}</span>{/if}
					</button>
					<button class="nav-item" class:active={activeSection === 'security'} onclick={() => activeSection = 'security'}>
						<span class="nav-label">Security</span>
						{#if sectionBadge('security') > 0}<span class="nav-badge">{sectionBadge('security')}</span>{/if}
					</button>
					<button class="nav-item" class:active={activeSection === 'performance'} onclick={() => activeSection = 'performance'}>
						<span class="nav-label">Performance</span>
						{#if sectionBadge('performance') > 0}<span class="nav-badge">{sectionBadge('performance')}</span>{/if}
					</button>
					<button class="nav-item" class:active={activeSection === 'routing'} onclick={() => activeSection = 'routing'}>
						<span class="nav-label">Routing</span>
						{#if sectionBadge('routing') > 0}<span class="nav-badge">{sectionBadge('routing')}</span>{/if}
					</button>
					<button class="nav-item" class:active={activeSection === 'locations'} onclick={() => activeSection = 'locations'}>
						<span class="nav-label">Locations</span>
						{#if sectionBadge('locations') > 0}<span class="nav-badge">{sectionBadge('locations')}</span>{/if}
					</button>
					<button class="nav-item" class:active={activeSection === 'loadbalancing'} onclick={() => activeSection = 'loadbalancing'}>
						<span class="nav-label">Load Balancing</span>
						{#if sectionBadge('loadbalancing') > 0}<span class="nav-badge">{sectionBadge('loadbalancing')}</span>{/if}
					</button>
					<button class="nav-item" class:active={activeSection === 'advanced'} onclick={() => activeSection = 'advanced'}>
						<span class="nav-label">Advanced</span>
						{#if sectionBadge('advanced') > 0}<span class="nav-badge">{sectionBadge('advanced')}</span>{/if}
					</button>
					{#if !isNew}
						<button class="nav-item" class:active={activeSection === 'history'} onclick={() => { activeSection = 'history'; if (!backupsLoaded) loadBackups(); }}>
							<span class="nav-label">History</span>
						</button>
					{/if}
				</div>
				<div class="nav-actions">
					<button class="btn-save" onclick={save} disabled={(!formUpstream.trim() && !config.redirect_only) || saving}>
						{#if saving}
							<svg class="spinner" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 2v4M12 18v4M4.93 4.93l2.83 2.83M16.24 16.24l2.83 2.83M2 12h4M18 12h4M4.93 19.07l2.83-2.83M16.24 7.76l2.83-2.83"/></svg>
							Saving...
						{:else}
							{isNew ? 'Create & Deploy' : 'Save & Deploy'}
						{/if}
					</button>
					<button class="btn-cancel" onclick={() => goto(`/hosts/${providerId}`)}>Cancel</button>
				</div>
			</nav>
			<div class="editor-content">
				{#if activeSection === 'basic'}
					<h2 class="content-title">Basic Configuration</h2>
					<p class="content-desc">The essentials: where traffic comes from and where it goes.</p>

					<div class="field">
						<label for="host-domain">Domain</label>
						<p class="field-desc">The subdomain visitors will use to reach your service. Use <code>@</code> for the root domain.</p>
						<div class="domain-input-wrap">
							<input id="host-domain" type="text" bind:value={formDomain} placeholder="app"
								oninput={() => { if (formSSL && certsLoaded) autoMatchCert(); }} />
							<span class="domain-suffix">.{parentDomain}</span>
						</div>
					</div>

					{#if !config.redirect_only}
						<div class="field">
							<label for="host-upstream">Upstream URL</label>
							<p class="field-desc">The internal address of your backend server. This is where nginx will forward requests.</p>
							<input id="host-upstream" type="text" bind:value={formUpstream} placeholder="http://127.0.0.1:3000" />
						</div>
					{/if}

					<div class="toggle-grid">
						<div class="toggle-card">
							<div class="toggle-card-text">
								<span class="toggle-card-label">SSL / HTTPS</span>
								<span class="toggle-card-desc">Encrypt traffic between visitors and your server with a TLS certificate.</span>
							</div>
							<button class="toggle-btn" class:active={formSSL} onclick={onSSLToggle} aria-label="Toggle SSL" type="button">
								<span class="toggle-track"><span class="toggle-thumb"></span></span>
							</button>
						</div>
						<div class="toggle-card">
							<div class="toggle-card-text">
								<span class="toggle-card-label">WebSocket</span>
								<span class="toggle-card-desc">Enable persistent two-way connections for real-time apps like chat or live updates.</span>
							</div>
							<button class="toggle-btn" class:active={formWebSocket} onclick={() => formWebSocket = !formWebSocket} aria-label="Toggle WebSocket" type="button">
								<span class="toggle-track"><span class="toggle-thumb"></span></span>
							</button>
						</div>
						<div class="toggle-card" class:disabled={!formSSL}>
							<div class="toggle-card-text">
								<span class="toggle-card-label">HTTP/2</span>
								<span class="toggle-card-desc">{formSSL ? 'Faster page loads through multiplexing and header compression. Recommended on.' : 'HTTP/2 requires SSL to be enabled.'}</span>
							</div>
							<button class="toggle-btn" class:active={config.http2 && formSSL} onclick={() => { if (formSSL) config.http2 = !config.http2; }} aria-label="Toggle HTTP/2" type="button" disabled={!formSSL}>
								<span class="toggle-track"><span class="toggle-thumb"></span></span>
							</button>
						</div>
						<div class="toggle-card">
							<div class="toggle-card-text">
								<span class="toggle-card-label">Redirect Only</span>
								<span class="toggle-card-desc">Don't proxy traffic -- just redirect visitors to another URL. No upstream needed.</span>
							</div>
							<button class="toggle-btn" class:active={config.redirect_only} onclick={() => config.redirect_only = !config.redirect_only} aria-label="Toggle Redirect Only" type="button">
								<span class="toggle-track"><span class="toggle-thumb"></span></span>
							</button>
						</div>
					</div>

					<div class="field">
						<label for="host-agent">Deploy to Agent</label>
						<p class="field-desc">Choose which server agent will receive and apply this nginx configuration. Leave unset for manual management.</p>
						<select id="host-agent" bind:value={formAgentId} onchange={(e) => { const t = e.target as HTMLSelectElement; formAgentId = t.value ? Number(t.value) : null; }}>
							<option value={null}>No agent (manual)</option>
							{#each agents as agent}
								<option value={agent.id}>{agent.name} ({agent.status})</option>
							{/each}
						</select>
					</div>

					<div class="sub">
						<div class="sub-header">
							<div>
								<h3>Server Aliases</h3>
								<p class="field-desc">Additional domain names that should serve the same content. Each alias becomes a <code>server_name</code> entry.</p>
							</div>
							<button class="btn-sm" type="button" onclick={addServerAlias}>+ Add</button>
						</div>
						{#each serverAliasesList as item, i}
							<div class="inline-row">
								<input type="text" bind:value={item.value} placeholder="alias.example.com" />
								<button class="btn-remove" type="button" aria-label="Remove" onclick={() => removeServerAlias(i)}>
									<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
								</button>
							</div>
						{/each}
						{#if serverAliasesList.length === 0}
							<p class="empty-hint">No aliases configured. Only the primary domain will be served.</p>
						{/if}
					</div>

					{#if formSSL}
						<div class="field">
							<!-- svelte-ignore a11y_label_has_associated_control -->
						<label>SSL Certificate</label>
							<p class="field-desc">The TLS certificate used to encrypt connections. Proxera auto-matches certificates based on domain name.</p>
							{#if matchedCert && formCertificateId === matchedCert.id}
								<div class="cert-match">
									<div class="cert-match-icon">
										<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"/><polyline points="22 4 12 14.01 9 11.01"/></svg>
									</div>
									<div class="cert-info">
										<span class="cert-domain">{matchedCert.domain}{matchedCert.san ? `, ${matchedCert.san}` : ''}</span>
										<span class="cert-expiry">Expires {new Date(matchedCert.expires_at ?? '').toLocaleDateString()}</span>
									</div>
									<button class="btn-sm" type="button" onclick={() => { matchedCert = null; }}>Change</button>
								</div>
							{:else if certificates.length > 0}
								<select bind:value={formCertificateId} onchange={(e) => { const t = e.target as HTMLSelectElement; formCertificateId = t.value ? Number(t.value) : null; }}>
									<option value={null}>Select a certificate...</option>
									{#each certificates as cert}
										<option value={cert.id}>{cert.domain}{cert.san ? ` + ${cert.san}` : ''} (expires {new Date(cert.expires_at ?? '').toLocaleDateString()})</option>
									{/each}
								</select>
							{:else if certsLoaded}
								<div class="cert-empty">No certificates available. <a href="/certificates">Issue a certificate</a> first.</div>
							{:else}
								<p class="empty-hint">Loading certificates...</p>
							{/if}
						</div>
					{/if}
				{/if}

				{#if activeSection === 'headers'}
					<h2 class="content-title">Headers & Authentication</h2>
					<p class="content-desc">Control HTTP headers sent to and from your upstream, and add password protection.</p>

					<!-- Proxy Headers -->
					<div class="sub">
						<h3>Proxy Headers</h3>
						<p class="field-desc">Headers added to every request forwarded to your upstream. These pass visitor and request information that would otherwise be lost behind the reverse proxy.</p>
						<div class="option-list">
							{#each DEFAULT_PROXY_HEADERS as hdr}
								<div class="option-row" class:active={!config.disabled_proxy_headers.includes(hdr.key)}>
									<div class="option-text">
										<span class="option-label"><code>{hdr.key}</code></span>
										<span class="option-desc">{hdr.desc}</span>
									</div>
									<button class="toggle-btn" class:active={!config.disabled_proxy_headers.includes(hdr.key)} onclick={() => toggleDefaultProxyHeader(hdr.key)} aria-label="Toggle {hdr.key}" type="button">
										<span class="toggle-track"><span class="toggle-thumb"></span></span>
									</button>
								</div>
							{/each}
							{#each EXTRA_PROXY_HEADERS as hdr}
								<div class="option-row" class:active={!!config.headers[hdr.key]}>
									<div class="option-text">
										<span class="option-label"><code>{hdr.key}</code></span>
										<span class="option-desc">{hdr.desc}</span>
									</div>
									<button class="toggle-btn" class:active={!!config.headers[hdr.key]} onclick={() => toggleProxyHeader(hdr)} aria-label="Toggle {hdr.key}" type="button">
										<span class="toggle-track"><span class="toggle-thumb"></span></span>
									</button>
								</div>
							{/each}
						</div>
					</div>

					<!-- Hide Upstream Headers -->
					<div class="sub">
						<h3>Hide Upstream Headers</h3>
						<p class="field-desc">Strip these headers from upstream responses before they reach the visitor. Hides information about your backend stack, reducing your attack surface.</p>
						<div class="option-list">
							{#each DEFAULT_HIDE_HEADERS as hdr}
								<div class="option-row" class:active={!config.disabled_hide_headers.includes(hdr.key)}>
									<div class="option-text">
										<span class="option-label"><code>{hdr.key}</code></span>
										<span class="option-desc">{hdr.desc}</span>
									</div>
									<button class="toggle-btn" class:active={!config.disabled_hide_headers.includes(hdr.key)} onclick={() => toggleDefaultHideHeader(hdr.key)} aria-label="Toggle {hdr.key}" type="button">
										<span class="toggle-track"><span class="toggle-thumb"></span></span>
									</button>
								</div>
							{/each}
							{#each EXTRA_HIDE_HEADERS as hdr}
								<div class="option-row" class:active={config.hide_headers.includes(hdr.key)}>
									<div class="option-text">
										<span class="option-label"><code>{hdr.key}</code></span>
										<span class="option-desc">{hdr.desc}</span>
									</div>
									<button class="toggle-btn" class:active={config.hide_headers.includes(hdr.key)} onclick={() => toggleHideHeader(hdr.key)} aria-label="Toggle {hdr.key}" type="button">
										<span class="toggle-track"><span class="toggle-thumb"></span></span>
									</button>
								</div>
							{/each}
						</div>
					</div>

					<!-- Basic Authentication -->
					<div class="sub">
						<div class="sub-header">
							<div>
								<h3>Basic Authentication</h3>
								<p class="field-desc">Require a username and password before visitors can access the site. The browser will show a native login prompt. Passwords are bcrypt-hashed on the agent.</p>
							</div>
							{#if !config.basic_auth}
								<button class="btn-sm" type="button" onclick={initBasicAuth}>Enable</button>
							{:else}
								<button class="btn-sm btn-sm-danger" type="button" onclick={clearBasicAuth}>Disable</button>
							{/if}
						</div>
						{#if config.basic_auth}
							<div class="form-row">
								<div class="field">
									<label for="host-basic-user">Username</label>
									<input id="host-basic-user" type="text" bind:value={config.basic_auth.username} placeholder="admin" />
								</div>
								<div class="field">
									<label for="host-basic-pass">Password</label>
									<input id="host-basic-pass" type="password" bind:value={config.basic_auth.password} placeholder="password" />
								</div>
							</div>
						{/if}
					</div>
				{/if}

				{#if activeSection === 'security'}
					<h2 class="content-title">Security</h2>
					<p class="content-desc">Rate limiting, IP access control, HSTS, CORS, and browser security headers.</p>

					<!-- Rate Limiting -->
					<div class="sub">
						<div class="sub-header">
							<div>
								<h3>Rate Limiting</h3>
								<p class="field-desc">Limit how many requests a single IP can make per second. Protects against abuse, brute-force attacks, and accidental traffic spikes.</p>
							</div>
							{#if !config.rate_limit}
								<button class="btn-sm" type="button" onclick={initRateLimit}>Enable</button>
							{:else}
								<button class="btn-sm btn-sm-danger" type="button" onclick={clearRateLimit}>Disable</button>
							{/if}
						</div>
						{#if config.rate_limit}
							<div class="form-row">
								<div class="field">
									<!-- svelte-ignore a11y_label_has_associated_control -->
								<label>Requests per second</label>
									<p class="field-desc">Steady-state rate. Requests beyond this are delayed or rejected.</p>
									<div class="chip-group">
										{#each RATE_PRESETS as rate}
											<button type="button" class="chip" class:active={config.rate_limit!.requests_per_second === rate}
												onclick={() => config.rate_limit!.requests_per_second = rate}>
												{rate}/s
											</button>
										{/each}
									</div>
								</div>
								<div class="field">
									<!-- svelte-ignore a11y_label_has_associated_control -->
								<label>Burst allowance</label>
									<p class="field-desc">Extra requests allowed in short bursts before rate limiting kicks in.</p>
									<div class="chip-group">
										{#each BURST_PRESETS as burst}
											<button type="button" class="chip" class:active={config.rate_limit!.burst === burst}
												onclick={() => config.rate_limit!.burst = burst}>
												{burst}
											</button>
										{/each}
									</div>
								</div>
							</div>
						{/if}
					</div>

					<!-- Trusted Proxies -->
					<div class="sub">
						<div class="sub-header">
							<div>
								<h3>Trusted Proxies</h3>
								<p class="field-desc">If traffic arrives through an upstream proxy (e.g. a homelab NAT, Cloudflare, or internal load balancer), add its IP or CIDR here. Nginx will read the real visitor IP from <code>X-Forwarded-For</code> instead of the proxy's IP. Only trust IPs you control.</p>
							</div>
							<button class="btn-sm" type="button" onclick={addTrustedProxy}>+ Add</button>
						</div>
						{#each trustedProxiesList as item, i}
							<div class="inline-row">
								<input type="text" bind:value={item.value} placeholder="10.0.0.0/8 or 192.168.1.1" />
								<button class="btn-remove" type="button" aria-label="Remove" onclick={() => removeTrustedProxy(i)}>
									<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
								</button>
							</div>
						{/each}
						{#if trustedProxiesList.length === 0}
							<p class="empty-hint">No trusted proxies set. <code>$remote_addr</code> is used as-is.</p>
						{/if}
					</div>

					<!-- Cloudflare Real IP -->
					<div class="sub">
						<div class="sub-header">
							<div>
								<h3>Cloudflare Real IP</h3>
								<p class="field-desc">Enable when Cloudflare proxy (orange cloud) is active. Automatically trusts all Cloudflare IP ranges and uses the <code>CF-Connecting-IP</code> header to extract the real visitor IP. Without this, metrics and logs will show Cloudflare edge IPs.</p>
							</div>
							<button class="toggle-btn" class:active={config.cloudflare_real_ip} onclick={() => config.cloudflare_real_ip = !config.cloudflare_real_ip} type="button">
								{config.cloudflare_real_ip ? 'On' : 'Off'}
							</button>
						</div>
					</div>

					<!-- HTTP Proxy Mode -->
					<div class="sub">
						<div class="sub-header">
							<div>
								<h3>HTTP Proxy Mode</h3>
								<p class="field-desc">Enable if this host receives traffic via an upstream proxy on port 80 (e.g. a Cloudflare-terminated reverse proxy forwarding plain HTTP internally). Nginx will proxy and log on port 80 instead of redirecting to HTTPS. Requires at least one trusted proxy IP above.</p>
							</div>
							<button class="toggle-btn" class:active={config.http_proxy} onclick={() => config.http_proxy = !config.http_proxy} type="button">
								{config.http_proxy ? 'On' : 'Off'}
							</button>
						</div>
					</div>

					<!-- IP Allowlist -->
					<div class="sub">
						<div class="sub-header">
							<div>
								<h3>IP Allowlist</h3>
								<p class="field-desc">Only allow traffic from these IPs or CIDR ranges. All other IPs will be blocked. Leave empty to allow everyone.</p>
							</div>
							<button class="btn-sm" type="button" onclick={addAllowlistIP}>+ Add</button>
						</div>
						{#each allowlistList as item, i}
							<div class="inline-row">
								<input type="text" bind:value={item.value} placeholder="192.168.1.0/24" />
								<button class="btn-remove" type="button" aria-label="Remove" onclick={() => removeAllowlistIP(i)}>
									<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
								</button>
							</div>
						{/each}
						{#if allowlistList.length === 0}
							<p class="empty-hint">No allowlist set. All IPs can connect.</p>
						{/if}
					</div>

					<!-- IP Blocklist -->
					<div class="sub">
						<div class="sub-header">
							<div>
								<h3>IP Blocklist</h3>
								<p class="field-desc">Block specific IPs or CIDR ranges from accessing this host. Useful for banning known bad actors.</p>
								{#if allowlistList.filter(i => i.value.trim()).length > 0}
									<p class="field-desc field-warn">An IP allowlist is active. The blocklist has no effect when an allowlist is set, since all non-allowed IPs are already blocked.</p>
								{/if}
							</div>
							<button class="btn-sm" type="button" onclick={addBlocklistIP}>+ Add</button>
						</div>
						{#each blocklistList as item, i}
							<div class="inline-row">
								<input type="text" bind:value={item.value} placeholder="10.0.0.1" />
								<button class="btn-remove" type="button" aria-label="Remove" onclick={() => removeBlocklistIP(i)}>
									<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
								</button>
							</div>
						{/each}
						{#if blocklistList.length === 0}
							<p class="empty-hint">No IPs blocked.</p>
						{/if}
					</div>

					<!-- HSTS -->
					<div class="sub">
						<div class="sub-header">
							<div>
								<h3>HSTS (HTTP Strict Transport Security)</h3>
								<p class="field-desc">Tell browsers to always use HTTPS for this domain. Once enabled, browsers will refuse to connect over plain HTTP for the configured duration.</p>
							</div>
							{#if !config.hsts}
								<button class="btn-sm" type="button" onclick={initHSTS}>Enable</button>
							{:else}
								<button class="btn-sm btn-sm-danger" type="button" onclick={() => config.hsts = null}>Disable</button>
							{/if}
						</div>
						{#if config.hsts}
							<div class="field">
								<!-- svelte-ignore a11y_label_has_associated_control -->
								<label>Max Age</label>
								<p class="field-desc">How long browsers should remember to only use HTTPS. Longer is more secure but harder to undo.</p>
								<div class="chip-group">
									{#each HSTS_PRESETS as preset}
										<button type="button" class="chip" class:active={config.hsts!.max_age === preset.value}
											onclick={() => config.hsts!.max_age = preset.value}>
											{preset.label}
										</button>
									{/each}
								</div>
							</div>
							<div class="form-row">
								<div class="toggle-card compact">
									<div class="toggle-card-text">
										<span class="toggle-card-label">Include Subdomains</span>
										<span class="toggle-card-desc">Apply HSTS to all subdomains too.</span>
									</div>
									<button class="toggle-btn" class:active={config.hsts!.include_subdomains} onclick={() => config.hsts!.include_subdomains = !config.hsts!.include_subdomains} aria-label="Toggle Include Subdomains" type="button">
										<span class="toggle-track"><span class="toggle-thumb"></span></span>
									</button>
								</div>
								<div class="toggle-card compact">
									<div class="toggle-card-text">
										<span class="toggle-card-label">Preload</span>
										<span class="toggle-card-desc">Allow inclusion in browser preload lists for HSTS.</span>
									</div>
									<button class="toggle-btn" class:active={config.hsts!.preload} onclick={() => config.hsts!.preload = !config.hsts!.preload} aria-label="Toggle Preload" type="button">
										<span class="toggle-track"><span class="toggle-thumb"></span></span>
									</button>
								</div>
							</div>
						{/if}
					</div>

					<!-- CORS -->
					<div class="sub">
						<div class="sub-header">
							<div>
								<h3>CORS (Cross-Origin Resource Sharing)</h3>
								<p class="field-desc">Allow other websites to make API requests to this domain. Required when your frontend and API are on different domains.</p>
							</div>
							{#if !config.cors}
								<button class="btn-sm" type="button" onclick={initCORS}>Enable</button>
							{:else}
								<button class="btn-sm btn-sm-danger" type="button" onclick={() => config.cors = null}>Disable</button>
							{/if}
						</div>
						{#if config.cors}
							<div class="form-row">
								<div class="toggle-card compact">
									<div class="toggle-card-text">
										<span class="toggle-card-label">Dynamic Origin</span>
										<span class="toggle-card-desc">Mirror the request's Origin header in the response. Required when using credentials.</span>
									</div>
									<button class="toggle-btn" class:active={config.cors!.dynamic} onclick={() => config.cors!.dynamic = !config.cors!.dynamic} aria-label="Toggle Dynamic Origin" type="button">
										<span class="toggle-track"><span class="toggle-thumb"></span></span>
									</button>
								</div>
								<div class="toggle-card compact">
									<div class="toggle-card-text">
										<span class="toggle-card-label">Allow Credentials</span>
										<span class="toggle-card-desc">Let browsers send cookies and auth headers in cross-origin requests.</span>
									</div>
									<button class="toggle-btn" class:active={config.cors!.allow_credentials} onclick={() => config.cors!.allow_credentials = !config.cors!.allow_credentials} aria-label="Toggle Allow Credentials" type="button">
										<span class="toggle-track"><span class="toggle-thumb"></span></span>
									</button>
								</div>
							</div>
							{#if !config.cors!.dynamic}
								<div class="sub nested">
									<div class="sub-header">
										<div>
											<!-- svelte-ignore a11y_label_has_associated_control -->
										<label>Allowed Origins</label>
											<p class="field-desc">Which domains can make cross-origin requests. Use <code>*</code> to allow any origin (incompatible with credentials).</p>
											{#if corsOriginsList.filter(i => i.value.trim() && i.value.trim() !== '*').length > 1}
												<p class="field-desc field-warn">Multiple origins require Dynamic Origin mode. It will be enabled automatically on save.</p>
											{/if}
										</div>
										<button class="btn-sm" type="button" onclick={addCorsOrigin}>+ Add</button>
									</div>
									{#each corsOriginsList as item, i}
										<div class="inline-row">
											<input type="text" bind:value={item.value} placeholder="https://example.com" />
											<button class="btn-remove" type="button" aria-label="Remove" onclick={() => removeCorsOrigin(i)}>
												<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
											</button>
										</div>
									{/each}
									{#if corsOriginsList.length === 0}
										<p class="empty-hint">No origins set. Defaults to * (any origin).</p>
									{/if}
								</div>
							{/if}
							<div class="field">
								<!-- svelte-ignore a11y_label_has_associated_control -->
								<label>Allowed Methods</label>
								<p class="field-desc">HTTP methods that cross-origin requests are allowed to use.</p>
								<div class="chip-group">
									{#each HTTP_METHODS as method}
										<button type="button" class="chip" class:active={config.cors!.allowed_methods?.includes(method)}
											onclick={() => config.cors!.allowed_methods = toggleChip(config.cors!.allowed_methods || [], method)}>
											{method}
										</button>
									{/each}
								</div>
							</div>
							<div class="field">
								<!-- svelte-ignore a11y_label_has_associated_control -->
								<label>Allowed Headers</label>
								<p class="field-desc">HTTP headers that cross-origin requests can include. Select <code>* (All)</code> to allow any header.</p>
								<div class="chip-group">
									<button type="button" class="chip" class:active={config.cors!.allowed_headers?.includes('*')}
										onclick={() => config.cors!.allowed_headers = config.cors!.allowed_headers?.includes('*') ? [] : ['*']}>
										* (All)
									</button>
									{#if !config.cors!.allowed_headers?.includes('*')}
										{#each CORS_HEADERS as hdr}
											<button type="button" class="chip" class:active={config.cors!.allowed_headers?.includes(hdr)}
												onclick={() => config.cors!.allowed_headers = toggleChip(config.cors!.allowed_headers || [], hdr)}>
												{hdr}
											</button>
										{/each}
									{/if}
								</div>
							</div>
							<div class="field">
								<!-- svelte-ignore a11y_label_has_associated_control -->
								<label>Preflight Cache (Max Age)</label>
								<p class="field-desc">How long browsers can cache preflight (OPTIONS) responses. Longer values reduce extra requests.</p>
								<div class="chip-group">
									{#each CORS_MAX_AGE_PRESETS as preset}
										<button type="button" class="chip" class:active={config.cors!.max_age === preset.value}
											onclick={() => config.cors!.max_age = preset.value}>
											{preset.label}
										</button>
									{/each}
								</div>
							</div>
						{/if}
					</div>

					<!-- Security Headers -->
					<div class="sub">
						<div class="sub-header">
							<div>
								<h3>Security Headers</h3>
								<p class="field-desc">Add browser security headers to protect against clickjacking, data leaks, and unwanted API access. These headers instruct the browser on how to handle your content.</p>
							</div>
							{#if !config.security_headers}
								<button class="btn-sm" type="button" onclick={initSecurityHeaders}>Configure</button>
							{:else}
								<button class="btn-sm btn-sm-danger" type="button" onclick={() => config.security_headers = null}>Reset</button>
							{/if}
						</div>
						{#if config.security_headers}
							<div class="field">
								<!-- svelte-ignore a11y_label_has_associated_control -->
								<label>X-Frame-Options</label>
								<p class="field-desc">Controls whether this page can be embedded in an iframe. <code>DENY</code> blocks all framing, <code>SAMEORIGIN</code> allows framing by your own site only.</p>
								<div class="chip-group">
									{#each [{l:'SAMEORIGIN',v:'SAMEORIGIN'},{l:'DENY',v:'DENY'}] as opt}
										<button type="button" class="chip" class:active={config.security_headers!.x_frame_options === opt.v}
											onclick={() => config.security_headers!.x_frame_options = opt.v}>
											{opt.l}
										</button>
									{/each}
								</div>
							</div>
							<div class="field">
								<!-- svelte-ignore a11y_label_has_associated_control -->
								<label>Referrer Policy</label>
								<p class="field-desc">Controls how much referrer URL info is sent when navigating away. Stricter policies protect user privacy but may break analytics.</p>
								<div class="chip-group">
									{#each ['no-referrer', 'strict-origin', 'strict-origin-when-cross-origin', 'same-origin', 'origin'] as pol}
										<button type="button" class="chip" class:active={config.security_headers!.referrer_policy === pol}
											onclick={() => config.security_headers!.referrer_policy = pol}>
											{pol}
										</button>
									{/each}
								</div>
							</div>
							<div class="field">
								<!-- svelte-ignore a11y_label_has_associated_control -->
								<label>Permissions Policy</label>
								<p class="field-desc">Block browser features you don't use. Selected features below will be <strong>disabled</strong> for this site, reducing your attack surface.</p>
								<div class="chip-group">
									{#each PERMISSIONS as perm}
										<button type="button" class="chip chip-deny" class:active={parsePermissionsPolicy(config.security_headers!.permissions_policy).includes(perm)}
											onclick={() => {
												const current = parsePermissionsPolicy(config.security_headers!.permissions_policy);
												config.security_headers!.permissions_policy = buildPermissionsPolicy(toggleChip(current, perm));
											}}>
											{perm}
										</button>
									{/each}
								</div>
							</div>
							<div class="field">
								<label for="host-csp">Content Security Policy (CSP)</label>
								<p class="field-desc">Define which resources (scripts, styles, images, etc.) the browser is allowed to load. A strong CSP is one of the best defenses against XSS attacks.</p>
								<input id="host-csp" type="text" bind:value={config.security_headers!.csp} placeholder="default-src 'self'; script-src 'self' 'unsafe-inline'" />
							</div>
						{/if}
					</div>
				{/if}

				{#if activeSection === 'performance'}
					<h2 class="content-title">Performance</h2>
					<p class="content-desc">Timeouts, compression, buffering, and connection tuning.</p>

					<!-- Timeouts -->
					<div class="sub">
						<div class="sub-header">
							<div>
								<h3>Proxy Timeouts</h3>
								<p class="field-desc">How long nginx will wait for your upstream server. Defaults to 60s for all three. Increase for slow APIs or large file uploads.</p>
							</div>
							{#if !config.timeouts}
								<button class="btn-sm" type="button" onclick={initTimeouts}>Configure</button>
							{:else}
								<button class="btn-sm btn-sm-danger" type="button" onclick={() => config.timeouts = null}>Reset</button>
							{/if}
						</div>
						{#if config.timeouts}
							<div class="form-row three">
								<div class="field">
									<!-- svelte-ignore a11y_label_has_associated_control -->
									<label>Connect</label>
									<p class="field-desc">Time to establish a connection to the upstream.</p>
									<div class="chip-group">
										{#each TIMEOUT_PRESETS as t}
											<button type="button" class="chip" class:active={config.timeouts!.connect === t.value}
												onclick={() => config.timeouts!.connect = t.value}>{t.label}</button>
										{/each}
									</div>
								</div>
								<div class="field">
									<!-- svelte-ignore a11y_label_has_associated_control -->
									<label>Send</label>
									<p class="field-desc">Time to transmit the request to the upstream.</p>
									<div class="chip-group">
										{#each TIMEOUT_PRESETS as t}
											<button type="button" class="chip" class:active={config.timeouts!.send === t.value}
												onclick={() => config.timeouts!.send = t.value}>{t.label}</button>
										{/each}
									</div>
								</div>
								<div class="field">
									<!-- svelte-ignore a11y_label_has_associated_control -->
									<label>Read</label>
									<p class="field-desc">Time to wait for the upstream's response.</p>
									<div class="chip-group">
										{#each TIMEOUT_PRESETS as t}
											<button type="button" class="chip" class:active={config.timeouts!.read === t.value}
												onclick={() => config.timeouts!.read = t.value}>{t.label}</button>
										{/each}
									</div>
								</div>
							</div>
						{/if}
					</div>

					<!-- Client Max Body Size -->
					<div class="sub">
						<h3>Client Max Body Size</h3>
						<p class="field-desc">Maximum size of a request body (file uploads, POST data). Requests exceeding this limit receive a <code>413 Request Entity Too Large</code> error. Default allows unlimited.</p>
						<div class="chip-group">
							{#each BODY_SIZE_PRESETS as size}
								<button type="button" class="chip" class:active={config.client_max_body_size === size}
									onclick={() => config.client_max_body_size = size}>
									{size || 'Default'}
								</button>
							{/each}
						</div>
					</div>

					<!-- Gzip -->
					<div class="sub">
						<div class="sub-header">
							<div>
								<h3>Gzip Compression</h3>
								<p class="field-desc">Compress responses before sending them to visitors. Reduces bandwidth and speeds up page loads, especially for text-based content.</p>
							</div>
							{#if !config.gzip}
								<button class="btn-sm" type="button" onclick={initGzip}>Enable</button>
							{:else}
								<button class="btn-sm btn-sm-danger" type="button" onclick={() => config.gzip = null}>Disable</button>
							{/if}
						</div>
						{#if config.gzip}
							<div class="toggle-card compact">
								<div class="toggle-card-text">
									<span class="toggle-card-label">Compression</span>
									<span class="toggle-card-desc">Toggle gzip on or off. When off, nginx sends an explicit <code>gzip off</code> directive.</span>
								</div>
								<button class="toggle-btn" class:active={config.gzip!.enabled} onclick={() => config.gzip!.enabled = !config.gzip!.enabled} aria-label="Toggle Compression" type="button">
									<span class="toggle-track"><span class="toggle-thumb"></span></span>
								</button>
							</div>
							{#if config.gzip.enabled}
								<div class="field">
									<!-- svelte-ignore a11y_label_has_associated_control -->
									<label>MIME Types</label>
									<p class="field-desc">Which content types to compress. Text and code formats benefit the most.</p>
									<div class="chip-group">
										{#each GZIP_TYPES as gtype}
											<button type="button" class="chip" class:active={config.gzip!.types?.includes(gtype.value)}
												onclick={() => config.gzip!.types = toggleChip(config.gzip!.types || [], gtype.value)}>
												{gtype.label}
											</button>
										{/each}
									</div>
								</div>
								<div class="form-row">
									<div class="field">
										<!-- svelte-ignore a11y_label_has_associated_control -->
										<label>Minimum Size</label>
										<p class="field-desc">Don't compress responses smaller than this. Compressing tiny files wastes CPU.</p>
										<div class="chip-group">
											{#each GZIP_MINLEN_PRESETS as p}
												<button type="button" class="chip" class:active={config.gzip!.min_length === p.value}
													onclick={() => config.gzip!.min_length = p.value}>{p.label}</button>
											{/each}
										</div>
									</div>
									<div class="field">
										<!-- svelte-ignore a11y_label_has_associated_control -->
										<label>Compression Level</label>
										<p class="field-desc">Higher levels compress more but use more CPU. Level 4-6 is a good balance.</p>
										<div class="chip-group">
											{#each GZIP_LEVEL_PRESETS as p}
												<button type="button" class="chip" class:active={config.gzip!.comp_level === p.value}
													onclick={() => config.gzip!.comp_level = p.value}>{p.label}</button>
											{/each}
										</div>
									</div>
								</div>
							{/if}
						{/if}
					</div>

					<!-- Proxy Buffering -->
					<div class="sub">
						<div class="sub-header">
							<div>
								<h3>Proxy Buffering</h3>
								<p class="field-desc">Buffer upstream responses in memory before sending to the client. Keeps upstream connections short-lived. Disable for server-sent events (SSE) or streaming responses.</p>
							</div>
							{#if !config.proxy_buffering}
								<button class="btn-sm" type="button" onclick={initProxyBuffering}>Configure</button>
							{:else}
								<button class="btn-sm btn-sm-danger" type="button" onclick={() => config.proxy_buffering = null}>Reset</button>
							{/if}
						</div>
						{#if config.proxy_buffering}
							<div class="toggle-card compact">
								<div class="toggle-card-text">
									<span class="toggle-card-label">Buffering</span>
									<span class="toggle-card-desc">When off, responses stream directly from upstream to client with no buffering.</span>
								</div>
								<button class="toggle-btn" class:active={config.proxy_buffering!.enabled} onclick={() => config.proxy_buffering!.enabled = !config.proxy_buffering!.enabled} aria-label="Toggle Buffering" type="button">
									<span class="toggle-track"><span class="toggle-thumb"></span></span>
								</button>
							</div>
							{#if config.proxy_buffering.enabled}
								<div class="form-row three">
									<div class="field">
										<!-- svelte-ignore a11y_label_has_associated_control -->
										<label>Buffer Size</label>
										<p class="field-desc">Size of a single buffer for the first part of the response (headers).</p>
										<div class="chip-group">
											{#each BUFFER_SIZE_PRESETS as s}
												<button type="button" class="chip" class:active={config.proxy_buffering!.buffer_size === s}
													onclick={() => config.proxy_buffering!.buffer_size = s}>{s}</button>
											{/each}
										</div>
									</div>
									<div class="field">
										<!-- svelte-ignore a11y_label_has_associated_control -->
										<label>Buffer Count</label>
										<p class="field-desc">Number of buffers allocated for the response body.</p>
										<div class="chip-group">
											{#each BUFFER_COUNT_PRESETS as c}
												<button type="button" class="chip" class:active={config.proxy_buffering!.buffers_count === c}
													onclick={() => config.proxy_buffering!.buffers_count = c}>{c}</button>
											{/each}
										</div>
									</div>
									<div class="field">
										<!-- svelte-ignore a11y_label_has_associated_control -->
										<label>Body Buffer Size</label>
										<p class="field-desc">Size of each body buffer.</p>
										<div class="chip-group">
											{#each BUFFER_SIZE_PRESETS as s}
												<button type="button" class="chip" class:active={config.proxy_buffering!.buffers_size === s}
													onclick={() => config.proxy_buffering!.buffers_size = s}>{s}</button>
											{/each}
										</div>
									</div>
								</div>
							{/if}
						{/if}
					</div>

					<!-- Request Buffering -->
					<div class="sub">
						<div class="toggle-card compact">
							<div class="toggle-card-text">
								<span class="toggle-card-label">Request Buffering</span>
								<span class="toggle-card-desc">Buffer the entire client request before forwarding to upstream. Disable this for streaming uploads or large file transfers where you want data sent to upstream immediately.</span>
							</div>
							<button class="toggle-btn" class:active={config.proxy_request_buffering !== false} onclick={() => {
								if (config.proxy_request_buffering === false) { config.proxy_request_buffering = null; }
								else { config.proxy_request_buffering = false; }
							}} aria-label="Toggle Request Buffering" type="button">
								<span class="toggle-track"><span class="toggle-thumb"></span></span>
							</button>
						</div>
					</div>

					<!-- Proxy SSL -->
					<div class="sub">
						<div class="sub-header">
							<div>
								<h3>Upstream SSL (HTTPS Backend)</h3>
								<p class="field-desc">Configure how nginx connects to upstream servers that use HTTPS. Only needed when your upstream URL starts with <code>https://</code>.</p>
							</div>
							{#if !config.proxy_ssl}
								<button class="btn-sm" type="button" onclick={initProxySSL}>Configure</button>
							{:else}
								<button class="btn-sm btn-sm-danger" type="button" onclick={() => config.proxy_ssl = null}>Reset</button>
							{/if}
						</div>
						{#if config.proxy_ssl}
							<div class="form-row">
								<div class="toggle-card compact">
									<div class="toggle-card-text">
										<span class="toggle-card-label">SSL Verify</span>
										<span class="toggle-card-desc">Validate the upstream's SSL certificate. Turn off for self-signed certs in development.</span>
									</div>
									<button class="toggle-btn" class:active={config.proxy_ssl!.verify} onclick={() => config.proxy_ssl!.verify = !config.proxy_ssl!.verify} aria-label="Toggle SSL Verify" type="button">
										<span class="toggle-track"><span class="toggle-thumb"></span></span>
									</button>
								</div>
								<div class="toggle-card compact">
									<div class="toggle-card-text">
										<span class="toggle-card-label">Server Name (SNI)</span>
										<span class="toggle-card-desc">Pass the domain name during the TLS handshake. Required when upstreams serve multiple domains.</span>
									</div>
									<button class="toggle-btn" class:active={config.proxy_ssl!.server_name} onclick={() => config.proxy_ssl!.server_name = !config.proxy_ssl!.server_name} aria-label="Toggle Server Name" type="button">
										<span class="toggle-track"><span class="toggle-thumb"></span></span>
									</button>
								</div>
							</div>
						{/if}
					</div>

	
					<!-- Access Log -->
					<div class="sub">
						<div class="toggle-card compact">
							<div class="toggle-card-text">
								<span class="toggle-card-label">Access Log</span>
								<span class="toggle-card-desc">Log every request to this host. Disable to reduce disk I/O on high-traffic sites, but you'll lose request visibility.</span>
							</div>
							<button class="toggle-btn" class:active={config.access_log} onclick={() => config.access_log = !config.access_log} aria-label="Toggle Access Log" type="button">
								<span class="toggle-track"><span class="toggle-thumb"></span></span>
							</button>
						</div>
					</div>
				{/if}

				{#if activeSection === 'routing'}
					<h2 class="content-title">Routing & Responses</h2>
					<p class="content-desc">URL redirects and custom error pages.</p>

					<!-- URL Redirects -->
					<div class="sub">
						<div class="sub-header">
							<div>
								<h3>URL Redirects</h3>
								<p class="field-desc">Redirect visitors from one path to another. Useful for moved pages, vanity URLs, or migrating from old paths. Choose 301 for permanent moves (search engines update their index) or 302 for temporary ones.</p>
							</div>
							<button class="btn-sm" type="button" onclick={addRedirect}>+ Add</button>
						</div>
						{#each redirectsList as item, i}
							<div class="list-item">
								<div class="inline-row">
									<input type="text" bind:value={item.source} placeholder="/old-path" />
									<span class="arrow">
										<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="5" y1="12" x2="19" y2="12"/><polyline points="12 5 19 12 12 19"/></svg>
									</span>
									<input type="text" bind:value={item.target} placeholder="/new-path or https://..." />
									<button class="btn-remove" type="button" aria-label="Remove" onclick={() => removeRedirect(i)}>
										<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
									</button>
								</div>
								<div class="chip-group" style="margin-top: 0.375rem">
									{#each REDIRECT_CODES as rc}
										<button type="button" class="chip chip-sm" class:active={item.code === rc.value}
											onclick={() => item.code = rc.value}>{rc.label}</button>
									{/each}
								</div>
							</div>
						{/each}
						{#if redirectsList.length === 0}
							<p class="empty-hint">No redirects. All requests go directly to the upstream.</p>
						{/if}
					</div>

					<!-- Custom Error Pages -->
					<div class="sub">
						<div class="sub-header">
							<div>
								<h3>Custom Error Pages</h3>
								<p class="field-desc">Show custom HTML pages for specific HTTP errors instead of the default nginx error page. The path should point to an HTML file on the server.</p>
							</div>
							<button class="btn-sm" type="button" onclick={addErrorPage}>+ Add</button>
						</div>
						{#each errorPagesList as item, i}
							<div class="list-item">
								<div class="inline-row">
									<input type="text" bind:value={item.path} placeholder="/custom_404.html" />
									<button class="btn-remove" type="button" aria-label="Remove" onclick={() => removeErrorPage(i)}>
										<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
									</button>
								</div>
								<div class="chip-group" style="margin-top: 0.375rem">
									{#each ERROR_CODES as code}
										<button type="button" class="chip chip-sm" class:active={item.code === code}
											onclick={() => item.code = code}>{code}</button>
									{/each}
								</div>
							</div>
						{/each}
						{#if errorPagesList.length === 0}
							<p class="empty-hint">No custom error pages. Nginx defaults will be used.</p>
						{/if}
					</div>

				{/if}

				{#if activeSection === 'locations'}
					<h2 class="content-title">Locations</h2>
					<p class="content-desc">Route specific URL paths to different backends. Each location is configured independently.</p>

					{#if !formSSL}
						<p class="field-desc field-warn">Locations only apply to SSL-enabled hosts. Enable SSL on the Basic tab to use this feature.</p>
					{/if}

					<div class="sub">
						<div class="sub-header">
							<div>
								<h3>Locations</h3>
								<p class="field-desc">Map URL paths to upstream servers. For example, send <code>/api</code> to one service and everything else to the main upstream.</p>
							</div>
							<button class="btn-sm" type="button" onclick={addLocation}>+ Add</button>
						</div>
						{#each locationsList as item, i}
							<div class="list-item">
								<div class="inline-row">
									<div class="reorder-btns">
										<button type="button" class="btn-reorder" onclick={() => moveLocation(i, -1)} disabled={i === 0} aria-label="Move up">
											<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><polyline points="18 15 12 9 6 15"/></svg>
										</button>
										<button type="button" class="btn-reorder" onclick={() => moveLocation(i, 1)} disabled={i === locationsList.length - 1} aria-label="Move down">
											<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><polyline points="6 9 12 15 18 9"/></svg>
										</button>
									</div>
									<input type="text" bind:value={item.path} placeholder="/api" style="flex:0.7" />
									<span class="arrow">
										<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="5" y1="12" x2="19" y2="12"/><polyline points="12 5 19 12 12 19"/></svg>
									</span>
									<input type="text" bind:value={item.upstream_url} placeholder="http://127.0.0.1:3001" />
									<button class="btn-remove" type="button" aria-label="Remove" onclick={() => removeLocation(i)}>
										<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
									</button>
								</div>
								<div class="option-list" style="margin-top: 0.5rem;">
									<div class="option-row" class:active={item.websocket}>
										<div class="option-text">
											<span class="option-label">WebSocket Support</span>
											<span class="option-desc">Enable WebSocket and HTTP upgrade proxying for this location.</span>
										</div>
										<button class="toggle-btn" class:active={item.websocket} onclick={() => item.websocket = !item.websocket} type="button" aria-label="Toggle WebSocket">
											<span class="toggle-track"><span class="toggle-thumb"></span></span>
										</button>
									</div>
									<div class="option-row" class:active={item.exact}>
										<div class="option-text">
											<span class="option-label">Exact Match</span>
											<span class="option-desc">Use <code>location = /path</code> — matches only this exact URL, not sub-paths. Takes priority over all prefix matches.</span>
										</div>
										<button class="toggle-btn" class:active={item.exact} onclick={() => item.exact = !item.exact} type="button" aria-label="Toggle Exact Match">
											<span class="toggle-track"><span class="toggle-thumb"></span></span>
										</button>
									</div>
									<div class="option-row" class:active={item.strip_prefix}>
										<div class="option-text">
											<span class="option-label">Strip Path Prefix</span>
											<span class="option-desc">Remove the location path before forwarding. E.g., <code>/api/users</code> is proxied as <code>/users</code>.</span>
										</div>
										<button class="toggle-btn" class:active={item.strip_prefix} onclick={() => item.strip_prefix = !item.strip_prefix} type="button" aria-label="Toggle Strip Prefix">
											<span class="toggle-track"><span class="toggle-thumb"></span></span>
										</button>
									</div>
									{#each DEFAULT_PROXY_HEADERS as hdr}
										<div class="option-row" class:active={!item._disabledProxyHeaders.includes(hdr.key)}>
											<div class="option-text">
												<span class="option-label"><code>{hdr.key}</code></span>
												<span class="option-desc">{hdr.desc}</span>
											</div>
											<button class="toggle-btn" class:active={!item._disabledProxyHeaders.includes(hdr.key)}
												onclick={() => {
													if (item._disabledProxyHeaders.includes(hdr.key)) {
														item._disabledProxyHeaders = item._disabledProxyHeaders.filter(h => h !== hdr.key);
													} else {
														item._disabledProxyHeaders = [...item._disabledProxyHeaders, hdr.key];
													}
												}}
												type="button" aria-label="Toggle {hdr.key}">
												<span class="toggle-track"><span class="toggle-thumb"></span></span>
											</button>
										</div>
									{/each}
									<div class="option-row" class:active={item._headerPairs?.some(p => p.key)}>
										<div class="option-text">
											<span class="option-label">Custom Request Headers</span>
											<span class="option-desc">Inject additional headers into requests forwarded to this upstream.</span>
											{#if item._expanded}
												<div style="margin-top: 0.5rem; display: flex; flex-direction: column; gap: 6px;">
													{#each item._headerPairs as pair, j}
														<div class="inline-row" style="gap: 6px;">
															<input type="text" bind:value={pair.key} placeholder="Header-Name" style="flex:0.45" />
															<input type="text" bind:value={pair.value} placeholder="value or nginx variable (e.g. $host)" style="flex:1" />
															<button class="btn-remove" type="button" aria-label="Remove header" onclick={() => item._headerPairs = item._headerPairs.filter((_, idx) => idx !== j)}>
																<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
															</button>
														</div>
													{/each}
													<button class="btn-sm" type="button" style="align-self: flex-start;" onclick={() => item._headerPairs = [...(item._headerPairs || []), { key: '', value: '' }]}>+ Add Header</button>
												</div>
											{/if}
										</div>
										<button class="toggle-btn" class:active={item._expanded} onclick={() => { item._expanded = !item._expanded; if (item._expanded && !(item._headerPairs?.length)) item._headerPairs = [{ key: '', value: '' }]; }} type="button" aria-label="Toggle Custom Headers">
											<span class="toggle-track"><span class="toggle-thumb"></span></span>
										</button>
									</div>
								</div>
							</div>
						{/each}
						{#if locationsList.length === 0}
							<p class="empty-hint">No locations configured. All requests go to the main upstream.</p>
						{/if}
					</div>
				{/if}

				{#if activeSection === 'loadbalancing'}
					<h2 class="content-title">Load Balancing</h2>
					<p class="content-desc">Distribute traffic across multiple backend servers for redundancy and scale.</p>

					<div class="sub">
						<div class="sub-header">
							<div>
								<h3>Upstream Servers</h3>
								<p class="field-desc">Define multiple backend servers to share the load. When enabled, the servers below replace the main upstream URL as the proxy target.</p>
							</div>
							{#if !config.load_balancing}
								<button class="btn-sm" type="button" onclick={initLoadBalancing}>Enable</button>
							{:else}
								<button class="btn-sm btn-sm-danger" type="button" onclick={() => { config.load_balancing = null; lbServersList = []; }}>Disable</button>
							{/if}
						</div>
						{#if config.load_balancing}
							<div class="field">
								<!-- svelte-ignore a11y_label_has_associated_control -->
								<label>Balancing Method</label>
								<p class="field-desc">
									<strong>Round Robin</strong> distributes requests evenly. <strong>Least Connections</strong> sends to the server with fewest active connections. <strong>IP Hash</strong> ensures the same client always reaches the same server (sticky sessions).
								</p>
								<div class="chip-group">
									{#each [{l:'Round Robin',v:'round-robin'},{l:'Least Connections',v:'least-conn'},{l:'IP Hash',v:'ip-hash'}] as m}
										<button type="button" class="chip" class:active={config.load_balancing!.method === m.v}
											onclick={() => config.load_balancing!.method = m.v}>{m.l}</button>
									{/each}
								</div>
							</div>
							<div class="sub-header" style="margin-top: 1rem">
								<div>
									<!-- svelte-ignore a11y_label_has_associated_control -->
									<label>Servers</label>
									<p class="field-desc">Each server gets a weight -- higher weight means more traffic. A server with weight 3 gets 3x the requests of one with weight 1.</p>
								</div>
								<button class="btn-sm" type="button" onclick={addLBServer}>+ Add</button>
							</div>
							{#each lbServersList as item, i}
								<div class="list-item">
									<div class="inline-row">
										<input type="text" bind:value={item.address} placeholder="127.0.0.1:3000" />
										<button class="btn-remove" type="button" aria-label="Remove" onclick={() => removeLBServer(i)}>
											<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
										</button>
									</div>
									<div class="chip-group" style="margin-top: 0.375rem">
										<span class="chip-label">Weight:</span>
										{#each LB_WEIGHT_PRESETS as w}
											<button type="button" class="chip chip-sm" class:active={item.weight === w}
												onclick={() => item.weight = w}>{w}</button>
										{/each}
									</div>
								</div>
							{/each}
							{#if lbServersList.length === 0}
								<p class="empty-hint">Add servers to create a load-balanced upstream pool.</p>
							{/if}
						{/if}
					</div>
				{/if}

				{#if activeSection === 'advanced'}
					<h2 class="content-title">Advanced</h2>
					<p class="content-desc">Raw nginx configuration for anything the UI doesn't cover.</p>

					<div class="field">
						<label for="host-custom-nginx">Custom Nginx Directives</label>
						<p class="field-desc">Raw nginx configuration inserted directly into the <code>server &#123;&#125;</code> block. Use this for directives not available in the UI above. Syntax errors here can break nginx, so test carefully.</p>
						<textarea id="host-custom-nginx" bind:value={config.custom_nginx_config} rows="8" placeholder="# Example: add custom headers&#10;add_header X-Custom-Header &quot;value&quot;;&#10;&#10;# Example: proxy cache&#10;proxy_cache_valid 200 1h;"></textarea>
					</div>
				{:else if activeSection === 'history'}
					<h2 class="content-title">Config History</h2>
					<p class="content-desc">Nginx config snapshots saved before each deployment. Restore any backup to roll back to a previous configuration.</p>

					{#if restoreMsg}
						<div class="msg msg-success" style="margin-bottom:1rem">
							<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"/><polyline points="22 4 12 14.01 9 11.01"/></svg>
							{restoreMsg}
						</div>
					{/if}
					{#if restoreError}
						<div class="msg msg-error" style="margin-bottom:1rem">
							<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><line x1="15" y1="9" x2="9" y2="15"/><line x1="9" y1="9" x2="15" y2="15"/></svg>
							{restoreError}
						</div>
					{/if}

					{#if backupsLoading}
						<div class="backup-empty"><div class="loader"></div><span>Loading backups…</span></div>
					{:else if backupsLoaded && backups.length === 0}
						<div class="backup-empty">
							<svg width="32" height="32" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/></svg>
							<span>No backups yet. Backups are created automatically on each save.</span>
						</div>
					{:else if backupsLoaded}
						<div class="tbl-wrap">
							<table>
								<thead>
									<tr>
										<th>Filename</th>
										<th>Created</th>
										<th>Size</th>
										<th></th>
									</tr>
								</thead>
								<tbody>
									{#each backups as b}
										<tr>
											<td class="backup-filename">{b.filename}</td>
											<td class="backup-date">{formatBackupDate(b.timestamp ?? b.created_at)}</td>
											<td class="backup-size">{formatBytes(b.size_bytes ?? b.size)}</td>
											<td class="backup-action">
												<button class="btn-view" onclick={() => openBackupViewer(b)}>View</button>
												<button
													class="btn-restore"
													disabled={restoring === b.filename}
													onclick={() => restoreBackup(b.filename)}
												>
													{#if restoring === b.filename}
														<svg class="spinner" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 2v4M12 18v4M4.93 4.93l2.83 2.83M16.24 16.24l2.83 2.83M2 12h4M18 12h4M4.93 19.07l2.83-2.83M16.24 7.76l2.83-2.83"/></svg>
														Restoring…
													{:else}
														Restore
													{/if}
												</button>
											</td>
										</tr>
									{/each}
								</tbody>
							</table>
						</div>
						<p class="backup-note">Up to 20 most recent backups are kept per host.</p>
					{/if}
				{/if}
			</div>
		</div>
	{/if}
</div>

{#if viewModalOpen}
	<!-- svelte-ignore a11y-click-events-have-key-events -->
	<!-- svelte-ignore a11y-no-static-element-interactions -->
	<div class="modal-backdrop" onclick={closeBackupViewer}>
		<div class="modal" onclick={(e) => e.stopPropagation()}>
			<div class="modal-head">
				<div class="modal-title-wrap">
					<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/></svg>
					<span class="modal-title">{viewModalFilename}</span>
				</div>
				<button class="modal-close" onclick={closeBackupViewer} aria-label="Close">
					<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
				</button>
			</div>
			<div class="modal-body">
				{#if viewModalLoading}
					<div class="modal-loading">
						<div class="loader"></div>
						<span>Loading config…</span>
					</div>
				{:else if viewModalError}
					<div class="msg msg-error">{viewModalError}</div>
				{:else}
					<pre class="config-preview">{viewModalContent}</pre>
				{/if}
			</div>
		</div>
	</div>
{/if}

<style>
	/* ── Layout ── */
	.page {
		padding: 2rem 2.25rem 3rem;
		margin: 0 auto;
	}
	.editor-layout {
		display: flex; gap: 1.5rem; align-items: flex-start;
	}
	.editor-content {
		flex: 1; min-width: 0;
	}
	.editor-nav {
		width: 180px; flex-shrink: 0;
		position: sticky; top: 1rem;
	}
	.nav-items {
		display: flex; flex-direction: column; gap: 0.125rem;
		margin-bottom: 1rem;
	}
	.nav-item {
		display: flex; align-items: center; justify-content: space-between; gap: 0.5rem;
		padding: 0.5rem 0.75rem;
		border: none; background: none; cursor: pointer;
		border-radius: var(--radius);
		font-size: var(--text-sm); font-weight: 500; font-family: inherit;
		color: var(--text-secondary);
		transition: all var(--transition);
		text-align: left; width: 100%;
		border-right: 2px solid transparent;
	}
	.nav-item:hover { background: var(--surface-raised); color: var(--text-primary); }
	.nav-item.active {
		border-right-color: var(--accent); color: var(--accent);
		background: var(--accent-dim); font-weight: 600;
	}
	.nav-badge {
		display: inline-flex; align-items: center; justify-content: center;
		min-width: 18px; height: 18px; padding: 0 5px;
		font-size: var(--text-xs); font-weight: 600; border-radius: 9px;
		background: var(--accent-dim); color: var(--accent);
	}
	.nav-item.active .nav-badge { background: rgba(255,255,255,0.15); }
	.nav-actions {
		display: flex; flex-direction: column; gap: 0.5rem;
		padding-top: 1rem; border-top: 1px solid var(--border);
	}
	.content-title {
		font-size: var(--text-base); font-weight: 700;
		color: var(--text-primary); margin: 0 0 0.25rem; letter-spacing: -0.01em;
	}
	.content-desc {
		font-size: var(--text-sm); color: var(--text-tertiary); margin: 0 0 1.25rem; line-height: 1.45;
	}

	/* ── Placeholder / Loading ── */
	.placeholder {
		display: flex; flex-direction: column; align-items: center;
		padding: 3rem; background: var(--surface);
		border: 1px solid var(--border); border-radius: var(--radius-lg); text-align: center;
	}
	.placeholder p { margin: 0.5rem 0 0; color: var(--text-secondary); font-size: var(--text-sm); }
	.loader {
		width: 100px; height: 2px; background: var(--border);
		border-radius: 1px; overflow: hidden; position: relative; margin-bottom: 0.75rem;
	}
	.loader::after {
		content: ''; position: absolute; top: 0; left: -40%;
		width: 40%; height: 100%; background: var(--accent);
		animation: slide 1.2s ease-in-out infinite;
	}
	@keyframes slide { 0% { left: -40%; } 100% { left: 100%; } }

	/* ── Page Header ── */
	.page-head { margin-bottom: 2rem; display: flex; flex-direction: column; gap: 0.5rem; }
	.breadcrumb {
		background: none; border: none; color: var(--accent);
		font-size: var(--text-xs); font-weight: 500; cursor: pointer;
		padding: 0; display: inline-flex; align-items: center; gap: 0.375rem;
		transition: color var(--transition);
	}
	.breadcrumb:hover { color: var(--accent-bright); }
	h1 {
		font-size: var(--text-2xl); font-weight: 700;
		color: var(--text-primary); letter-spacing: -0.02em; margin: 0;
	}
	.page-desc { color: var(--text-tertiary); margin: 0; font-size: var(--text-sm); line-height: 1.5; }

	/* ── Messages ── */
	.msg {
		display: flex; align-items: center; gap: 0.625rem;
		padding: 0.75rem 1rem; border-radius: var(--radius); margin-bottom: 1rem;
		font-size: var(--text-sm); line-height: 1.4;
	}
	.msg svg { flex-shrink: 0; }
	.msg-error { background: var(--danger-dim); border: 1px solid var(--danger); color: var(--danger); }
	.msg-success { background: var(--accent-dim); border: 1px solid var(--accent); color: var(--accent); }


	/* ── Sections (removed, content renders inline) ── */

	/* ── Subsections ── */
	.sub { margin-bottom: 1.5rem; }
	.sub:last-child { margin-bottom: 0; }
	.sub.nested { margin-left: 0; margin-top: 0.75rem; padding: 0.875rem; background: var(--surface-raised); border-radius: var(--radius); border: 1px solid var(--border); }
	.sub-header { display: flex; align-items: flex-start; justify-content: space-between; gap: 1rem; margin-bottom: 0.75rem; }
	.sub h3 { font-size: var(--text-sm); font-weight: 600; color: var(--text-secondary); margin: 0; }

	/* ── Fields ── */
	.field { margin-bottom: 1rem; }
	.field:last-child { margin-bottom: 0; }
	.field label {
		display: block; font-size: var(--text-xs); font-weight: 600; color: var(--text-secondary);
		text-transform: uppercase; letter-spacing: 0.04em; margin-bottom: 0.25rem;
	}
	.field-desc {
		font-size: var(--text-sm); color: var(--text-tertiary); margin: 0 0 0.5rem; line-height: 1.45;
	}
	.field-desc code {
		font-family: var(--font-mono); font-size: var(--text-xs);
		padding: 0.125rem 0.375rem; background: var(--surface-raised); border-radius: var(--radius); color: var(--text-secondary);
	}
	.field-desc strong { color: var(--text-secondary); font-weight: 600; }
	.field-warn { color: var(--warning); font-weight: 500; }

	.field input, .field select, .field textarea {
		width: 100%; padding: 0.625rem 0.75rem; border: 1px solid var(--border); border-radius: var(--radius);
		font-size: var(--text-sm); font-family: inherit; box-sizing: border-box; background: var(--bg); color: var(--text-primary);
		transition: border-color var(--transition);
	}
	.field textarea { font-family: var(--font-mono); font-size: var(--text-xs); resize: vertical; line-height: 1.5; }
	.field input:focus, .field select:focus, .field textarea:focus { outline: none; border-color: var(--accent); }
	.field input::placeholder, .field textarea::placeholder { color: var(--text-tertiary); }

	.form-row { display: flex; gap: 0.75rem; }
	.form-row.three { display: grid; grid-template-columns: 1fr 1fr 1fr; gap: 0.75rem; }

	.empty-hint { font-size: var(--text-sm); color: var(--text-tertiary); margin: 0; font-style: italic; }

	/* ── Domain Input ── */
	.domain-input-wrap {
		display: flex; align-items: center; border: 1px solid var(--border); border-radius: var(--radius); overflow: hidden;
		transition: border-color var(--transition);
	}
	.domain-input-wrap:focus-within { border-color: var(--accent); }
	.domain-input-wrap input { border: none !important; border-radius: 0 !important; box-shadow: none !important; }
	.domain-suffix {
		padding: 0.625rem 0.875rem; background: var(--surface-raised); color: var(--text-tertiary);
		font-size: var(--text-sm); white-space: nowrap; border-left: 1px solid var(--border);
		font-family: var(--font-mono);
	}

	/* ── Toggle Cards ── */
	.toggle-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 0.625rem; margin-bottom: 1rem; }
	.toggle-card {
		display: flex; align-items: center; justify-content: space-between; gap: 0.875rem;
		padding: 0.875rem 1rem; background: var(--surface-raised); border: 1px solid var(--border);
		border-radius: var(--radius); transition: border-color var(--transition);
	}
	.toggle-card.disabled { opacity: 0.5; }
	.toggle-card.disabled button { cursor: not-allowed; }
	.toggle-card.compact { margin-bottom: 0.75rem; }
	.toggle-card.compact:last-child { margin-bottom: 0; }
	.toggle-card-text { flex: 1; min-width: 0; }
	.toggle-card-label { display: block; font-size: var(--text-sm); font-weight: 600; color: var(--text-primary); margin-bottom: 0.125rem; }
	.toggle-card-desc { display: block; font-size: var(--text-xs); color: var(--text-tertiary); line-height: 1.4; }
	.toggle-card-desc code {
		font-family: var(--font-mono); font-size: var(--text-xs);
		padding: 0.0625rem 0.25rem; background: var(--bg); border-radius: var(--radius); color: var(--text-secondary);
	}

	/* ── Option List (Headers) ── */
	.option-list { display: flex; flex-direction: column; gap: 0; border: 1px solid var(--border); border-radius: var(--radius); overflow: hidden; }
	.option-row {
		display: flex; align-items: center; justify-content: space-between; gap: 1rem;
		padding: 0.75rem 1rem; border-bottom: 1px solid var(--border);
		transition: background var(--transition);
	}
	.option-row:last-child { border-bottom: none; }
	.option-row:hover { background: var(--surface-raised); }
	.option-row.active { background: var(--accent-dim); }
	.option-text { flex: 1; min-width: 0; }
	.option-label { display: block; margin-bottom: 0.125rem; }
	.option-label code {
		font-family: var(--font-mono); font-size: var(--text-sm); font-weight: 600;
		color: var(--text-primary);
	}
	.option-row.active .option-label code { color: var(--accent); }
	.option-desc { display: block; font-size: var(--text-xs); color: var(--text-tertiary); line-height: 1.4; }


	/* ── Toggle Button ── */
	.toggle-btn { display: flex; align-items: center; gap: 0.5rem; background: none; border: none; cursor: pointer; padding: 0; font-family: inherit; flex-shrink: 0; color: var(--text-secondary); }
	.toggle-track {
		width: 38px; height: 22px; background: var(--border-bright); border-radius: 11px;
		position: relative; transition: background var(--transition);
	}
	.toggle-btn.active .toggle-track { background: var(--accent); }
	.toggle-thumb {
		position: absolute; top: 2px; left: 2px; width: 18px; height: 18px; background: white;
		border-radius: 50%; transition: transform var(--transition); box-shadow: 0 1px 3px rgba(0, 0, 0, 0.25);
	}
	.toggle-btn.active .toggle-thumb { transform: translateX(16px); }
	.toggle-label { font-size: var(--text-xs); color: var(--text-secondary); font-weight: 500; }
	.toggle-btn.compact { padding: 0; flex-shrink: 0; }

	/* ── Chip Toggles ── */
	.chip-group { display: flex; flex-wrap: wrap; gap: 0.375rem; }
	.chip {
		padding: 0.3125rem 0.6875rem; border-radius: var(--radius); font-size: var(--text-xs); font-weight: 500;
		cursor: pointer; border: 1px solid var(--border); background: transparent;
		color: var(--text-tertiary); transition: all var(--transition); font-family: inherit;
	}
	.chip:hover { border-color: var(--border-bright); color: var(--text-secondary); }
	.chip.active { background: var(--accent-dim); border-color: var(--accent); color: var(--accent); }
	.chip.chip-deny.active { background: var(--danger-dim); border-color: var(--danger); color: var(--danger); }
	.chip.chip-sm { padding: 0.2rem 0.5rem; font-size: var(--text-xs); }
	.chip-label { font-size: var(--text-xs); color: var(--text-tertiary); align-self: center; font-weight: 500; }

	/* ── Inline Rows & List Items ── */
	.inline-row { display: flex; gap: 0.5rem; align-items: center; margin-bottom: 0.5rem; }
	.inline-row:last-child { margin-bottom: 0; }
	.inline-row input {
		flex: 1; padding: 0.5rem 0.6875rem; border: 1px solid var(--border); border-radius: var(--radius);
		font-size: var(--text-sm); font-family: inherit; box-sizing: border-box; background: var(--bg); color: var(--text-primary);
		transition: border-color var(--transition);
	}
	.inline-row input:focus { outline: none; border-color: var(--accent); }
	.arrow { color: var(--text-tertiary); flex-shrink: 0; display: flex; align-items: center; }

	.list-item {
		margin-bottom: 0.75rem; padding-bottom: 0.75rem;
		border-bottom: 1px solid var(--border);
	}
	.list-item:last-child { border-bottom: none; margin-bottom: 0; padding-bottom: 0; }

	/* ── Buttons ── */
	.btn-sm {
		padding: 0.3125rem 0.75rem; border-radius: var(--radius); font-size: var(--text-xs); font-weight: 500;
		cursor: pointer; border: 1px solid var(--border); background: transparent;
		color: var(--text-secondary); transition: all var(--transition); flex-shrink: 0; white-space: nowrap; font-family: inherit;
	}
	.btn-sm:hover { border-color: var(--accent); color: var(--accent); }
	.btn-sm-danger:hover { border-color: var(--danger); color: var(--danger); }

	.btn-remove {
		background: transparent; border: 1px solid var(--border); color: var(--text-tertiary);
		cursor: pointer; padding: 0.375rem; border-radius: var(--radius); display: flex; align-items: center; justify-content: center;
		flex-shrink: 0; transition: all var(--transition);
	}
	.btn-remove:hover { border-color: var(--danger); color: var(--danger); background: var(--danger-dim); }

	.reorder-btns { display: flex; flex-direction: column; gap: 1px; flex-shrink: 0; }
	.btn-reorder {
		background: transparent; border: 1px solid var(--border); color: var(--text-tertiary);
		cursor: pointer; padding: 0.2rem 0.3rem; border-radius: calc(var(--radius) / 2);
		display: flex; align-items: center; justify-content: center; line-height: 1;
		transition: all var(--transition);
	}
	.btn-reorder:hover:not(:disabled) { border-color: var(--accent); color: var(--accent); background: var(--accent-dim); }
	.btn-reorder:disabled { opacity: 0.25; cursor: default; }

	/* ── Certificates ── */
	.cert-match {
		display: flex; align-items: center; gap: 0.75rem; padding: 0.75rem 1rem;
		background: var(--accent-dim); border: 1px solid var(--accent); border-radius: var(--radius);
	}
	.cert-match-icon { color: var(--accent); flex-shrink: 0; display: flex; }
	.cert-info { flex: 1; display: flex; flex-direction: column; gap: 0.125rem; }
	.cert-domain { font-size: var(--text-sm); font-weight: 600; color: var(--text-primary); font-family: var(--font-mono); }
	.cert-expiry { font-size: var(--text-xs); color: var(--text-tertiary); }
	.cert-empty {
		font-size: var(--text-sm); color: var(--warning); padding: 0.75rem 1rem;
		background: var(--warning-dim); border: 1px solid var(--warning); border-radius: var(--radius);
	}
	.cert-empty a { color: var(--accent); text-decoration: underline; }

	/* ── Action Buttons ── */
	.btn-cancel {
		padding: 0.5rem 1.125rem; border-radius: var(--radius); font-size: var(--text-sm); font-weight: 600;
		cursor: pointer; border: 1px solid var(--border); background: transparent;
		color: var(--text-secondary); transition: all var(--transition); font-family: inherit;
		width: 100%; text-align: center;
	}
	.btn-cancel:hover { border-color: var(--border-bright); color: var(--text-primary); }
	.btn-save {
		display: inline-flex; align-items: center; justify-content: center; gap: 0.5rem;
		padding: 0.5rem 1.125rem; border-radius: var(--radius); font-size: var(--text-sm); font-weight: 600;
		cursor: pointer; border: none; background: var(--accent); color: #fff;
		transition: background var(--transition); font-family: inherit;
		width: 100%;
	}
	.btn-save:hover:not(:disabled) { background: var(--accent-bright); }
	.btn-save:disabled { opacity: 0.45; cursor: not-allowed; }

	.spinner { animation: spin 1s linear infinite; }
	@keyframes spin { to { transform: rotate(360deg); } }

	/* ── Responsive ── */
	@media (max-width: 900px) {
		.editor-layout { flex-direction: column; }
		.editor-nav {
			width: 100%; position: static;
			border-bottom: 1px solid var(--border); padding-bottom: 1rem;
		}
		.nav-items {
			flex-direction: row; flex-wrap: wrap; gap: 0.25rem;
			margin-bottom: 0.75rem;
		}
		.nav-item {
			border-right: none; border-bottom: 2px solid transparent;
			padding: 0.375rem 0.625rem; font-size: var(--text-xs);
		}
		.nav-item.active { border-right-color: transparent; border-bottom-color: var(--accent); }
		.nav-actions { flex-direction: row; border-top: none; padding-top: 0; }
		.nav-actions .btn-save, .nav-actions .btn-cancel { width: auto; }
	}
	@media (max-width: 768px) {
		.page { padding: 1.25rem; }
		h1 { font-size: var(--text-xl); }
		.toggle-grid { grid-template-columns: 1fr; }
		.form-row { flex-direction: column; gap: 0; }
		.form-row.three { grid-template-columns: 1fr; }
	}

	/* ── Backup History ── */
	.backup-empty {
		display: flex; flex-direction: column; align-items: center; justify-content: center;
		gap: 0.75rem; padding: 2.5rem 1rem;
		background: var(--surface); border: 1px solid var(--border); border-radius: var(--radius-lg);
		color: var(--text-tertiary); font-size: var(--text-sm); text-align: center;
	}
	.backup-filename {
		font-family: var(--font-mono); font-size: var(--text-xs);
		color: var(--text-primary); word-break: break-all;
	}
	.backup-date { font-size: var(--text-sm); color: var(--text-secondary); white-space: nowrap; }
	.backup-size { font-size: var(--text-sm); color: var(--text-tertiary); white-space: nowrap; }

	.btn-restore {
		display: inline-flex; align-items: center; gap: 0.375rem;
		padding: 0.3rem 0.75rem; border-radius: var(--radius); font-size: var(--text-xs);
		font-weight: 600; cursor: pointer; font-family: inherit;
		border: 1px solid var(--border); background: transparent; color: var(--text-secondary);
		transition: all var(--transition);
	}
	.btn-restore:hover:not(:disabled) { border-color: var(--accent); color: var(--accent); background: var(--accent-dim); }
	.btn-restore:disabled { opacity: 0.5; cursor: not-allowed; }
	.backup-note {
		font-size: var(--text-xs); color: var(--text-tertiary);
		margin: 0.75rem 0 0; text-align: right;
	}
	.backup-action { display: flex; gap: 0.375rem; justify-content: flex-end; }
	.btn-view {
		display: inline-flex; align-items: center; gap: 0.375rem;
		padding: 0.3rem 0.75rem; border-radius: var(--radius); font-size: var(--text-xs);
		font-weight: 600; cursor: pointer; font-family: inherit;
		border: 1px solid var(--border); background: transparent; color: var(--text-secondary);
		transition: all var(--transition);
	}
	.btn-view:hover { border-color: var(--border-bright); color: var(--text-primary); background: var(--surface-raised); }

	/* ── Backup Viewer Modal ── */
	.modal-backdrop {
		position: fixed; inset: 0; z-index: 1000;
		background: rgba(0,0,0,0.6); backdrop-filter: blur(2px);
		display: flex; align-items: center; justify-content: center;
		padding: 1.5rem;
	}
	.modal {
		background: var(--surface); border: 1px solid var(--border);
		border-radius: var(--radius-lg); width: 100%; max-width: 800px;
		max-height: 80vh; display: flex; flex-direction: column;
		box-shadow: 0 24px 64px rgba(0,0,0,0.4);
	}
	.modal-head {
		display: flex; align-items: center; justify-content: space-between;
		padding: 1rem 1.25rem; border-bottom: 1px solid var(--border); flex-shrink: 0;
	}
	.modal-title-wrap {
		display: flex; align-items: center; gap: 0.5rem;
		color: var(--text-secondary); min-width: 0;
	}
	.modal-title {
		font-size: var(--text-sm); font-weight: 600; font-family: var(--font-mono);
		color: var(--text-primary); white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
	}
	.modal-close {
		flex-shrink: 0; display: flex; align-items: center; justify-content: center;
		width: 28px; height: 28px; border-radius: var(--radius); border: none;
		background: transparent; color: var(--text-tertiary); cursor: pointer;
		transition: all var(--transition);
	}
	.modal-close:hover { background: var(--surface-raised); color: var(--text-primary); }
	.modal-body {
		flex: 1; overflow: auto; padding: 1.25rem;
	}
	.modal-loading {
		display: flex; flex-direction: column; align-items: center; justify-content: center;
		gap: 0.75rem; padding: 2rem; color: var(--text-tertiary); font-size: var(--text-sm);
	}
	.config-preview {
		margin: 0; font-family: var(--font-mono); font-size: var(--text-xs);
		line-height: 1.6; color: var(--text-primary);
		white-space: pre; overflow-x: auto;
	}
</style>
