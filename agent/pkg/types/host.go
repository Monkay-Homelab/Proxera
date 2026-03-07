package types

// Host represents a proxied domain/service
type Host struct {
	Domain      string            `yaml:"domain" json:"domain"`
	UpstreamURL string            `yaml:"upstream_url" json:"upstream_url"`
	SSL         bool              `yaml:"ssl" json:"ssl"`
	CertPath    string            `yaml:"cert_path,omitempty" json:"cert_path,omitempty"`
	KeyPath     string            `yaml:"key_path,omitempty" json:"key_path,omitempty"`
	Headers     map[string]string `yaml:"headers,omitempty" json:"headers,omitempty"`
	WebSocket   bool              `yaml:"websocket,omitempty" json:"websocket,omitempty"`
	BasicAuth   *BasicAuth        `yaml:"basic_auth,omitempty" json:"basic_auth,omitempty"`
	CertPEM     string            `yaml:"-" json:"cert_pem,omitempty"`
	KeyPEM      string            `yaml:"-" json:"key_pem,omitempty"`
	IssuerPEM   string            `yaml:"-" json:"issuer_pem,omitempty"`
	Config      *AdvancedConfig   `yaml:"-" json:"config,omitempty"`
}

// BasicAuth represents HTTP basic authentication
type BasicAuth struct {
	Username string `yaml:"username" json:"username"`
	Password string `yaml:"password" json:"password"`
}

// AdvancedConfig contains all advanced nginx configuration options
type AdvancedConfig struct {
	Headers              map[string]string      `json:"headers,omitempty"`
	HideHeaders          []string               `json:"hide_headers,omitempty"`
	DisabledProxyHeaders []string               `json:"disabled_proxy_headers,omitempty"`
	DisabledHideHeaders  []string               `json:"disabled_hide_headers,omitempty"`
	BasicAuth            *BasicAuth             `json:"basic_auth,omitempty"`
	RateLimit            *RateLimitConfig       `json:"rate_limit,omitempty"`
	IPAllowlist          []string               `json:"ip_allowlist,omitempty"`
	IPBlocklist          []string               `json:"ip_blocklist,omitempty"`
	HSTS                 *HSTSConfig            `json:"hsts,omitempty"`
	CORS                 *CORSConfig            `json:"cors,omitempty"`
	SecurityHeaders      *SecurityHeadersConfig `json:"security_headers,omitempty"`
	Timeouts             *TimeoutConfig         `json:"timeouts,omitempty"`
	ClientMaxBodySize    string                 `json:"client_max_body_size,omitempty"`
	Gzip                 *GzipConfig            `json:"gzip,omitempty"`
	ProxyBuffering       *ProxyBufferingConfig  `json:"proxy_buffering,omitempty"`
	ProxyRequestBuffering *bool                 `json:"proxy_request_buffering,omitempty"`
	ProxySSL             *ProxySSLConfig        `json:"proxy_ssl,omitempty"`
	Keepalive            int                    `json:"keepalive,omitempty"`
	Redirects            []RedirectConfig       `json:"redirects,omitempty"`
	CustomErrorPages     map[string]string      `json:"custom_error_pages,omitempty"`
	Locations            []LocationConfig       `json:"locations,omitempty"`
	LoadBalancing        *LoadBalancingConfig   `json:"load_balancing,omitempty"`
	CustomNginxConfig    string                 `json:"custom_nginx_config,omitempty"`
	HTTP2                *bool                  `json:"http2,omitempty"`
	AccessLog            *bool                  `json:"access_log,omitempty"`
	ServerAliases        []string               `json:"server_aliases,omitempty"`
	RedirectOnly         bool                   `json:"redirect_only,omitempty"`
	HTTPProxy            *bool                  `json:"http_proxy,omitempty"`
	TrustedProxies       []string               `json:"trusted_proxies,omitempty"`
	CloudflareRealIP     bool                   `json:"cloudflare_real_ip,omitempty"`
}

type RateLimitConfig struct {
	RequestsPerSecond int `json:"requests_per_second,omitempty"`
	Burst             int `json:"burst,omitempty"`
}

type HSTSConfig struct {
	Enabled           bool `json:"enabled"`
	MaxAge            int  `json:"max_age"`
	IncludeSubDomains bool `json:"include_subdomains"`
	Preload           bool `json:"preload"`
}

type CORSConfig struct {
	Enabled          bool     `json:"enabled"`
	Dynamic          bool     `json:"dynamic,omitempty"`
	AllowedOrigins   []string `json:"allowed_origins,omitempty"`
	AllowedMethods   []string `json:"allowed_methods,omitempty"`
	AllowedHeaders   []string `json:"allowed_headers,omitempty"`
	AllowCredentials bool     `json:"allow_credentials"`
	MaxAge           int      `json:"max_age,omitempty"`
}

type SecurityHeadersConfig struct {
	CSP               string `json:"csp,omitempty"`
	ReferrerPolicy    string `json:"referrer_policy,omitempty"`
	PermissionsPolicy string `json:"permissions_policy,omitempty"`
	XFrameOptions     string `json:"x_frame_options,omitempty"`
}

type TimeoutConfig struct {
	Connect int `json:"connect"`
	Send    int `json:"send"`
	Read    int `json:"read"`
}

type GzipConfig struct {
	Enabled   bool     `json:"enabled"`
	Types     []string `json:"types,omitempty"`
	MinLength int      `json:"min_length,omitempty"`
	CompLevel int      `json:"comp_level,omitempty"`
}

type ProxySSLConfig struct {
	Verify     bool `json:"verify"`
	ServerName bool `json:"server_name"`
}

type ProxyBufferingConfig struct {
	Enabled      bool   `json:"enabled"`
	BufferSize   string `json:"buffer_size,omitempty"`
	BuffersCount int    `json:"buffers_count,omitempty"`
	BuffersSize  string `json:"buffers_size,omitempty"`
}

type RedirectConfig struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Code   int    `json:"code"`
}

type LocationConfig struct {
	Path                   string            `json:"path"`
	UpstreamURL            string            `json:"upstream_url"`
	WebSocket              bool              `json:"websocket"`
	Exact                  bool              `json:"exact,omitempty"`
	StripPrefix            bool              `json:"strip_prefix,omitempty"`
	UseLocationProxyHeaders bool             `json:"use_location_proxy_headers,omitempty"`
	DisabledProxyHeaders   []string          `json:"disabled_proxy_headers,omitempty"`
	Headers                map[string]string `json:"headers,omitempty"`
}

type LoadBalancingConfig struct {
	Method  string               `json:"method"` // round-robin, least-conn, ip-hash
	Servers []LoadBalancingServer `json:"servers"`
}

type LoadBalancingServer struct {
	Address string `json:"address"`
	Weight  int    `json:"weight,omitempty"`
}

// AgentConfig represents the agent's configuration
type AgentConfig struct {
	// Agent settings
	AgentID   string `yaml:"agent_id,omitempty" json:"agent_id,omitempty"`
	AgentPort int    `yaml:"agent_port" json:"agent_port"` // Dynamic port range (52080 default)

	// Panel connection (optional for standalone mode)
	PanelURL string `yaml:"panel_url,omitempty" json:"panel_url,omitempty"`
	APIKey   string `yaml:"api_key,omitempty" json:"api_key,omitempty"`

	// Nginx paths
	NginxConfigPath  string `yaml:"nginx_config_path" json:"nginx_config_path"`
	NginxEnabledPath string `yaml:"nginx_enabled_path" json:"nginx_enabled_path"`
	NginxBinary      string `yaml:"nginx_binary" json:"nginx_binary"`

	// Metrics
	MetricsInterval int `yaml:"metrics_interval,omitempty" json:"metrics_interval,omitempty"` // seconds, default 300 (5 min)

	// Hosts to manage
	Hosts []Host `yaml:"hosts" json:"hosts"`
}
