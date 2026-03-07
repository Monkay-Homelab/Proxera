package nginx

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/proxera/agent/pkg/types"
)

// Validation patterns for nginx directive values
var (
	// domainRe matches valid DNS hostnames: labels separated by dots, each 1-63 chars (letters, digits, hyphens)
	domainRe = regexp.MustCompile(`^(\*\.)?([a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)*[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$`)

	// nginxSizeRe matches nginx size values like "1m", "512k", "8k", "100"
	nginxSizeRe = regexp.MustCompile(`^\d+[kmgKMG]?$`)

	// safePathRe matches safe URL paths (no semicolons, newlines, or nginx directive chars)
	safePathRe = regexp.MustCompile(`^/[a-zA-Z0-9_.~:/?#\[\]@!$&'()*+,;=%{}-]*$`)

	// safeMIMETypeRe matches MIME types like "text/plain", "application/json"
	safeMIMETypeRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9!#$&\-^_.+]*\/[a-zA-Z0-9][a-zA-Z0-9!#$&\-^_.+]*$`)

	// lbMethodRe matches valid load balancing methods
	lbMethodRe = regexp.MustCompile(`^(round-robin|least-conn|ip-hash)$`)

	// httpMethodRe matches valid HTTP methods
	httpMethodRe = regexp.MustCompile(`^(GET|POST|PUT|PATCH|DELETE|OPTIONS|HEAD)$`)

	// xFrameOptionsRe matches valid X-Frame-Options values
	xFrameOptionsRe = regexp.MustCompile(`^(DENY|SAMEORIGIN|ALLOW-FROM .+)$`)

	// referrerPolicyRe matches valid Referrer-Policy values
	referrerPolicyRe = regexp.MustCompile(`^(no-referrer|no-referrer-when-downgrade|origin|origin-when-cross-origin|same-origin|strict-origin|strict-origin-when-cross-origin|unsafe-url)$`)

	// redirectCodeRe matches valid redirect status codes
	validRedirectCodes = map[int]bool{301: true, 302: true, 303: true, 307: true, 308: true}
)

// containsNginxInjection checks for characters that could inject nginx directives
func containsNginxInjection(s string) bool {
	return strings.ContainsAny(s, ";\n\r{}")
}

// ValidateHost validates all user-controlled fields in a host config before template rendering.
// Returns an error describing the first invalid field found.
func ValidateHost(host *types.Host) error {
	// Validate domain
	if host.Domain == "" {
		return fmt.Errorf("domain is required")
	}
	if !domainRe.MatchString(host.Domain) {
		return fmt.Errorf("invalid domain format: %s", host.Domain)
	}

	// Validate upstream URL
	if host.UpstreamURL != "" {
		u, err := url.Parse(host.UpstreamURL)
		if err != nil || u.Host == "" {
			return fmt.Errorf("invalid upstream URL: %s", host.UpstreamURL)
		}
	}

	if host.Config == nil {
		return nil
	}

	cfg := host.Config

	// Validate server aliases
	for _, alias := range cfg.ServerAliases {
		if !domainRe.MatchString(alias) {
			return fmt.Errorf("invalid server alias: %s", alias)
		}
	}

	// Validate IP allowlist/blocklist
	for _, ip := range cfg.IPAllowlist {
		if err := validateIPOrCIDR(ip); err != nil {
			return fmt.Errorf("invalid IP allowlist entry %q: %w", ip, err)
		}
	}
	for _, ip := range cfg.IPBlocklist {
		if err := validateIPOrCIDR(ip); err != nil {
			return fmt.Errorf("invalid IP blocklist entry %q: %w", ip, err)
		}
	}

	// Validate trusted proxies
	for _, ip := range cfg.TrustedProxies {
		if err := validateIPOrCIDR(ip); err != nil {
			return fmt.Errorf("invalid trusted proxy entry %q: %w", ip, err)
		}
	}

	// Validate client_max_body_size
	if cfg.ClientMaxBodySize != "" {
		if !nginxSizeRe.MatchString(cfg.ClientMaxBodySize) {
			return fmt.Errorf("invalid client_max_body_size: %s", cfg.ClientMaxBodySize)
		}
		if err := validateMaxBodySize(cfg.ClientMaxBodySize); err != nil {
			return err
		}
	}

	// Validate proxy buffering sizes
	if cfg.ProxyBuffering != nil {
		if cfg.ProxyBuffering.BufferSize != "" && !nginxSizeRe.MatchString(cfg.ProxyBuffering.BufferSize) {
			return fmt.Errorf("invalid proxy buffer_size: %s", cfg.ProxyBuffering.BufferSize)
		}
		if cfg.ProxyBuffering.BuffersSize != "" && !nginxSizeRe.MatchString(cfg.ProxyBuffering.BuffersSize) {
			return fmt.Errorf("invalid proxy buffers_size: %s", cfg.ProxyBuffering.BuffersSize)
		}
	}

	// Validate gzip types
	if cfg.Gzip != nil {
		for _, t := range cfg.Gzip.Types {
			if !safeMIMETypeRe.MatchString(t) {
				return fmt.Errorf("invalid gzip MIME type: %s", t)
			}
		}
	}

	// Validate redirects
	for _, r := range cfg.Redirects {
		if r.Source == "" || !strings.HasPrefix(r.Source, "/") || containsNginxInjection(r.Source) {
			return fmt.Errorf("invalid redirect source: %s", r.Source)
		}
		if r.Target == "" || containsNginxInjection(r.Target) {
			return fmt.Errorf("invalid redirect target: %s", r.Target)
		}
		if !validRedirectCodes[r.Code] {
			return fmt.Errorf("invalid redirect code: %d", r.Code)
		}
	}

	// Validate custom error pages
	for code, path := range cfg.CustomErrorPages {
		if containsNginxInjection(code) || containsNginxInjection(path) {
			return fmt.Errorf("invalid custom error page: %s -> %s", code, path)
		}
		if !safePathRe.MatchString(path) && !strings.HasPrefix(path, "=") {
			return fmt.Errorf("invalid error page path: %s", path)
		}
	}

	// Validate load balancing
	if cfg.LoadBalancing != nil {
		if cfg.LoadBalancing.Method != "" && !lbMethodRe.MatchString(cfg.LoadBalancing.Method) {
			return fmt.Errorf("invalid load balancing method: %s", cfg.LoadBalancing.Method)
		}
		for _, srv := range cfg.LoadBalancing.Servers {
			if containsNginxInjection(srv.Address) {
				return fmt.Errorf("invalid load balancing server address: %s", srv.Address)
			}
			// Must be host:port or IP:port
			if _, _, err := net.SplitHostPort(srv.Address); err != nil {
				// Try without port (just IP or hostname)
				if net.ParseIP(srv.Address) == nil && !domainRe.MatchString(srv.Address) {
					return fmt.Errorf("invalid load balancing server address: %s", srv.Address)
				}
			}
		}
	}

	// Validate CORS origins
	if cfg.CORS != nil {
		for _, origin := range cfg.CORS.AllowedOrigins {
			if origin == "*" {
				continue
			}
			if containsNginxInjection(origin) {
				return fmt.Errorf("invalid CORS origin: %s", origin)
			}
			u, err := url.Parse(origin)
			if err != nil || u.Host == "" {
				return fmt.Errorf("invalid CORS origin URL: %s", origin)
			}
		}
		for _, method := range cfg.CORS.AllowedMethods {
			if !httpMethodRe.MatchString(method) {
				return fmt.Errorf("invalid CORS method: %s", method)
			}
		}
		for _, header := range cfg.CORS.AllowedHeaders {
			if header == "*" {
				continue
			}
			if containsNginxInjection(header) {
				return fmt.Errorf("invalid CORS header: %s", header)
			}
		}
	}

	// Validate security headers
	if cfg.SecurityHeaders != nil {
		if cfg.SecurityHeaders.XFrameOptions != "" &&
			!xFrameOptionsRe.MatchString(cfg.SecurityHeaders.XFrameOptions) {
			return fmt.Errorf("invalid X-Frame-Options: %s", cfg.SecurityHeaders.XFrameOptions)
		}
		if cfg.SecurityHeaders.ReferrerPolicy != "" &&
			!referrerPolicyRe.MatchString(cfg.SecurityHeaders.ReferrerPolicy) {
			return fmt.Errorf("invalid Referrer-Policy: %s", cfg.SecurityHeaders.ReferrerPolicy)
		}
		// CSP and Permissions-Policy are freeform but must not inject nginx directives
		if containsNginxInjection(cfg.SecurityHeaders.CSP) {
			return fmt.Errorf("invalid CSP: contains forbidden characters")
		}
		if containsNginxInjection(cfg.SecurityHeaders.PermissionsPolicy) {
			return fmt.Errorf("invalid Permissions-Policy: contains forbidden characters")
		}
	}

	// Validate locations
	for _, loc := range cfg.Locations {
		if loc.Path == "" || containsNginxInjection(loc.Path) {
			return fmt.Errorf("invalid location path: %s", loc.Path)
		}
		if loc.UpstreamURL != "" {
			u, err := url.Parse(loc.UpstreamURL)
			if err != nil || u.Host == "" {
				return fmt.Errorf("invalid location upstream URL: %s", loc.UpstreamURL)
			}
		}
		for k, v := range loc.Headers {
			if containsNginxInjection(k) || containsNginxInjection(v) {
				return fmt.Errorf("invalid location header: %s", k)
			}
		}
	}

	// Validate custom nginx config - block dangerous directives
	if cfg.CustomNginxConfig != "" {
		lower := strings.ToLower(cfg.CustomNginxConfig)
		dangerous := []string{"lua_", "load_module", "include ", "ssl_certificate", "root ", "alias "}
		for _, d := range dangerous {
			if strings.Contains(lower, d) {
				return fmt.Errorf("custom nginx config contains disallowed directive: %s", d)
			}
		}
	}

	// Validate custom headers
	for k, v := range cfg.Headers {
		if containsNginxInjection(k) || containsNginxInjection(v) {
			return fmt.Errorf("invalid custom header: %s", k)
		}
	}

	return nil
}

// validateMaxBodySize ensures client_max_body_size doesn't exceed 100g.
func validateMaxBodySize(size string) error {
	// Parse numeric part and optional suffix
	suffix := size[len(size)-1]
	var numStr string
	var multiplier int64

	switch suffix {
	case 'k', 'K':
		numStr = size[:len(size)-1]
		multiplier = 1024
	case 'm', 'M':
		numStr = size[:len(size)-1]
		multiplier = 1024 * 1024
	case 'g', 'G':
		numStr = size[:len(size)-1]
		multiplier = 1024 * 1024 * 1024
	default:
		numStr = size
		multiplier = 1 // bytes
	}

	num, err := strconv.ParseInt(numStr, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid client_max_body_size number: %s", size)
	}

	const maxBytes = 100 * 1024 * 1024 * 1024 // 100g
	if num*multiplier > maxBytes {
		return fmt.Errorf("client_max_body_size exceeds maximum of 100g: %s", size)
	}
	return nil
}

// validateIPOrCIDR validates an IP address or CIDR notation
func validateIPOrCIDR(s string) error {
	if strings.Contains(s, "/") {
		_, _, err := net.ParseCIDR(s)
		if err != nil {
			return fmt.Errorf("invalid CIDR: %w", err)
		}
		return nil
	}
	if net.ParseIP(s) == nil {
		return fmt.Errorf("invalid IP address")
	}
	return nil
}
