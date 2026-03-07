package handlers

// HostAdvancedConfig contains all advanced nginx configuration options
type HostAdvancedConfig struct {
	Headers              map[string]string      `json:"headers,omitempty"`
	HideHeaders          []string               `json:"hide_headers,omitempty"`
	DisabledProxyHeaders []string               `json:"disabled_proxy_headers,omitempty"`
	DisabledHideHeaders  []string               `json:"disabled_hide_headers,omitempty"`
	BasicAuth            *BasicAuthConfig       `json:"basic_auth,omitempty"`
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

type BasicAuthConfig struct {
	Username string `json:"username"`
	Password string `json:"password"`
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
	Path                    string            `json:"path"`
	UpstreamURL             string            `json:"upstream_url"`
	WebSocket               bool              `json:"websocket"`
	Exact                   bool              `json:"exact,omitempty"`
	StripPrefix             bool              `json:"strip_prefix,omitempty"`
	UseLocationProxyHeaders bool              `json:"use_location_proxy_headers,omitempty"`
	DisabledProxyHeaders    []string          `json:"disabled_proxy_headers,omitempty"`
	Headers                 map[string]string `json:"headers,omitempty"`
}

type LoadBalancingConfig struct {
	Method  string               `json:"method"` // round-robin, least-conn, ip-hash
	Servers []LoadBalancingServer `json:"servers"`
}

type LoadBalancingServer struct {
	Address string `json:"address"`
	Weight  int    `json:"weight,omitempty"`
}
