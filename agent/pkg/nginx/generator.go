package nginx

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"

	"github.com/proxera/agent/pkg/types"
)

// cloudflareIPRanges contains Cloudflare's published IP ranges.
// https://www.cloudflare.com/ips/
var cloudflareIPRanges = []string{
	// IPv4
	"173.245.48.0/20",
	"103.21.244.0/22",
	"103.22.200.0/22",
	"103.31.4.0/22",
	"141.101.64.0/18",
	"108.162.192.0/18",
	"190.93.240.0/20",
	"188.114.96.0/20",
	"197.234.240.0/22",
	"198.41.128.0/17",
	"162.158.0.0/15",
	"104.16.0.0/13",
	"104.24.0.0/14",
	"172.64.0.0/13",
	"131.0.72.0/22",
	// IPv6
	"2400:cb00::/32",
	"2606:4700::/32",
	"2803:f800::/32",
	"2405:b500::/32",
	"2405:8100::/32",
	"2a06:98c0::/29",
	"2c0f:f248::/32",
}

// SanitizeDomain converts a domain name to a safe filesystem identifier.
// Underscores are escaped to double-underscores first, then dots become single underscores.
// This ensures domains with underscores (e.g., "my_app.example.com") produce unique filenames.
func SanitizeDomain(domain string) string {
	s := strings.ReplaceAll(domain, "_", "__")
	return strings.ReplaceAll(s, ".", "_")
}

// UnsanitizeDomain reverses SanitizeDomain — converts a filesystem identifier back to a domain.
func UnsanitizeDomain(safe string) string {
	// Use a placeholder to avoid collision during multi-step replacement
	const placeholder = "\x00"
	s := strings.ReplaceAll(safe, "__", placeholder)
	s = strings.ReplaceAll(s, "_", ".")
	return strings.ReplaceAll(s, placeholder, "_")
}

// compareVersions compares two dotted version strings (e.g. "1.25.1" vs "1.24.0").
// Returns -1, 0, or 1.
func compareVersions(a, b string) int {
	partsA := strings.Split(a, ".")
	partsB := strings.Split(b, ".")
	maxLen := len(partsA)
	if len(partsB) > maxLen {
		maxLen = len(partsB)
	}
	for i := 0; i < maxLen; i++ {
		var va, vb int
		if i < len(partsA) {
			va, _ = strconv.Atoi(partsA[i])
		}
		if i < len(partsB) {
			vb, _ = strconv.Atoi(partsB[i])
		}
		if va < vb {
			return -1
		}
		if va > vb {
			return 1
		}
	}
	return 0
}

// DetectNginxVersion returns the installed nginx version (e.g. "1.28.2").
// Returns empty string if nginx is not installed or version cannot be determined.
// In Docker mode, it derives the container name from NGINX_TEST_CMD to query the right container.
func DetectNginxVersion() string {
	// Try local nginx first
	out, err := exec.Command("nginx", "-v").CombinedOutput()
	if err != nil {
		// Docker mode: extract container from NGINX_TEST_CMD (e.g. "docker exec project-nginx-1 nginx -t")
		testCmd := os.Getenv("NGINX_TEST_CMD")
		if testCmd != "" {
			parts := strings.Fields(testCmd)
			// Look for "docker exec <container> ..."
			for i, p := range parts {
				if p == "exec" && i+1 < len(parts) {
					container := parts[i+1]
					out, err = exec.Command("docker", "exec", container, "nginx", "-v").CombinedOutput()
					if err != nil {
						return ""
					}
					break
				}
			}
			if err != nil {
				return ""
			}
		} else {
			return ""
		}
	}
	s := strings.TrimSpace(string(out))
	if idx := strings.LastIndex(s, "/"); idx >= 0 {
		return s[idx+1:]
	}
	return ""
}

const nginxConfigTemplate = `# Proxera-managed configuration for {{ .Domain }}
# Generated automatically - do not edit manually
{{ if hasLoadBalancing . }}
upstream proxera_{{ sanitizeDomain .Domain }} {
    {{- if eq (lbMethod .) "least-conn" }}
    least_conn;
    {{- else if eq (lbMethod .) "ip-hash" }}
    ip_hash;
    {{- end }}
    {{- range lbServers . }}
    server {{ .Address }}{{ if .Weight }} weight={{ .Weight }}{{ end }};
    {{- end }}
}
{{ end }}{{ if hasRateLimit . }}
limit_req_zone $binary_remote_addr zone=proxera_{{ sanitizeDomain .Domain }}:10m rate={{ rateLimitRate . }}r/s;
{{ end }}
# HTTP server
server {
    listen 80;
    listen [::]:80;
    server_name {{ serverNames . }};
{{- if hasTrustedProxies . }}
    # Real IP from trusted upstream proxies
    {{- range trustedProxies . }}
    set_real_ip_from {{ . }};
    {{- end }}
    real_ip_header {{ realIPHeader . }};
    real_ip_recursive on;
{{ end }}
    # CrowdSec blocklist
    if ($crowdsec_blocklist) {
        return 403;
    }
{{ if .SSL }}
    # ACME Challenge
    location /.well-known/acme-challenge/ {
        alias /var/www/proxera-acme/.well-known/acme-challenge/;
        allow all;
    }
{{ if httpProxy . }}
{{ if hasClientMaxBody . }}
    client_max_body_size {{ clientMaxBody . }};
{{ end }}
{{ if hasDefaultSecurityHeaders . }}
    # Security headers
    add_header X-Frame-Options "{{ xFrameOptions . }}" always;
    add_header X-Content-Type-Options "nosniff" always;
    {{- if hasReferrerPolicy . }}
    add_header Referrer-Policy "{{ referrerPolicy . }}" always;
    {{- end }}
    {{- if hasPermissionsPolicy . }}
    add_header Permissions-Policy "{{ permissionsPolicy . }}" always;
    {{- end }}
{{ end }}
    # HTTP proxy (dual-mode)
    location / {
        {{- if hasLoadBalancing . }}
        proxy_pass http://proxera_{{ sanitizeDomain .Domain }};
        {{- else }}
        proxy_pass {{ .UpstreamURL }};
        {{- end }}
        proxy_http_version 1.1;
        {{- range defaultProxyHeaders . }}
        proxy_set_header {{ index . 0 }} {{ index . 1 }};
        {{- end }}

        {{- if .WebSocket }}
        # WebSocket support
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        {{- end }}

        {{- range hideHeaders . }}
        proxy_hide_header {{ . }};
        {{- end }}

        # Timeouts
        proxy_connect_timeout {{ connectTimeout . }}s;
        proxy_send_timeout {{ sendTimeout . }}s;
        proxy_read_timeout {{ readTimeout . }}s;
    }
    # Logging
    {{- if enableAccessLog . }}
    access_log /var/log/nginx/{{ sanitizeDomain .Domain }}_access.log proxera_metrics;
    access_log /var/log/nginx/crowdsec_access.log combined;
    {{- else }}
    access_log off;
    {{- end }}
    error_log /var/log/nginx/{{ sanitizeDomain .Domain }}_error.log;
{{ else }}
    location / {
        return 301 https://$server_name$request_uri;
    }
{{ end }}
}

# HTTPS server
server {
    listen 443 ssl{{ if and (enableHTTP2 .) (not (nginxVersionAtLeast "1.25.1")) }} http2{{ end }};
    listen [::]:443 ssl{{ if and (enableHTTP2 .) (not (nginxVersionAtLeast "1.25.1")) }} http2{{ end }};
{{- if and (enableHTTP2 .) (nginxVersionAtLeast "1.25.1") }}
    http2 on;
{{- end }}
    server_name {{ serverNames . }};
{{- if hasTrustedProxies . }}
    # Real IP from trusted upstream proxies
    {{- range trustedProxies . }}
    set_real_ip_from {{ . }};
    {{- end }}
    real_ip_header {{ realIPHeader . }};
    real_ip_recursive on;
{{ end }}
    # CrowdSec blocklist
    if ($crowdsec_blocklist) {
        return 403;
    }

    # SSL Configuration
    ssl_certificate {{ .CertPath }};
    ssl_certificate_key {{ .KeyPath }};
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384:ECDHE-ECDSA-CHACHA20-POLY1305:ECDHE-RSA-CHACHA20-POLY1305;
    ssl_prefer_server_ciphers off;
{{ else }}
{{ end }}
    {{- if hasHSTS . }}
    add_header Strict-Transport-Security "max-age={{ hstsMaxAge . }}{{ if hstsIncludeSubDomains . }}; includeSubDomains{{ end }}{{ if hstsPreload . }}; preload{{ end }}" always;
    {{- end }}
{{ if hasGzip . }}
    # Gzip compression
    gzip on;
    gzip_vary on;
    gzip_proxied any;
    {{- if gzipCompLevel . }}
    gzip_comp_level {{ gzipCompLevel . }};
    {{- end }}
    {{- if gzipMinLength . }}
    gzip_min_length {{ gzipMinLength . }};
    {{- end }}
    gzip_types {{ gzipTypes . }};
{{ else if isGzipOff . }}
    gzip off;
{{ end }}{{ if hasClientMaxBody . }}
    client_max_body_size {{ clientMaxBody . }};
{{ end }}
{{ if hasDefaultSecurityHeaders . }}
    # Security headers
    add_header X-Frame-Options "{{ xFrameOptions . }}" always;
    add_header X-Content-Type-Options "nosniff" always;
    {{- if hasReferrerPolicy . }}
    add_header Referrer-Policy "{{ referrerPolicy . }}" always;
    {{- end }}
    {{- if hasPermissionsPolicy . }}
    add_header Permissions-Policy "{{ permissionsPolicy . }}" always;
    {{- end }}
    {{- if hasCSP . }}
    add_header Content-Security-Policy "{{ csp . }}" always;
    {{- end }}
{{ end }}
    {{- if hasCORS . }}
    # CORS
    {{- if corsDynamic . }}
    add_header Access-Control-Allow-Origin $http_origin always;
    {{- else }}
    add_header Access-Control-Allow-Origin "{{ joinStrings (corsOrigins .) ", " }}" always;
    {{- end }}
    add_header Access-Control-Allow-Methods "{{ joinStrings (corsMethods .) ", " }}" always;
    add_header Access-Control-Allow-Headers "{{ joinStrings (corsHeaders .) ", " }}" always;
    {{- if corsCredentials . }}
    add_header Access-Control-Allow-Credentials "true" always;
    {{- end }}
    {{- if corsMaxAge . }}
    add_header Access-Control-Max-Age "{{ corsMaxAge . }}" always;
    {{- end }}

    # Handle preflight OPTIONS requests
    if ($request_method = 'OPTIONS') {
        return 204;
    }
    {{- end }}
{{ if hasIPAllowlist . }}
    # IP Allowlist
    {{- range ipAllowlist . }}
    allow {{ . }};
    {{- end }}
    deny all;
{{ end }}{{ if hasIPBlocklist . }}
    # IP Blocklist
    {{- range ipBlocklist . }}
    deny {{ . }};
    {{- end }}
{{ end }}{{ if hasBasicAuth . }}
    # Basic Authentication
    auth_basic "Restricted Access";
    auth_basic_user_file {{ htpasswdPath . }};
{{ end }}{{ if hasRedirects . }}
    # Redirects
    {{- range redirects . }}
    location = {{ .Source }} {
        return {{ .Code }} {{ .Target }};
    }
    {{- end }}
{{ end }}{{ if hasCustomErrorPages . }}
    # Custom error pages
    {{- range $code, $path := customErrorPages . }}
    error_page {{ $code }} {{ $path }};
    {{- end }}
{{ end }}{{ if hasLocations . }}
    # Path-based locations
    {{- range locations . }}
    location {{ if .Exact }}= {{ end }}{{ .Path }} {
        proxy_pass {{ locationProxyPass . }};
        proxy_http_version 1.1;
        {{- range locationProxyHeaders $ . }}
        proxy_set_header {{ index . 0 }} {{ index . 1 }};
        {{- end }}
        {{- if .WebSocket }}
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        {{- end }}
        {{- range $key, $val := mergedHeaders $ }}
        proxy_set_header {{ $key }} "{{ $val }}";
        {{- end }}
        {{- range $key, $val := .Headers }}
        proxy_set_header {{ $key }} {{ $val }};
        {{- end }}
        {{- range hideHeaders $ }}
        proxy_hide_header {{ . }};
        {{- end }}
    }
    {{- end }}
{{ end }}{{ if not (isRedirectOnly .) }}
    # Default location
    location / {
        {{- if hasCORS . }}
        # Strip CORS headers from upstream (nginx will add them)
        proxy_hide_header Access-Control-Allow-Origin;
        proxy_hide_header Access-Control-Allow-Methods;
        proxy_hide_header Access-Control-Allow-Headers;
        proxy_hide_header Access-Control-Allow-Credentials;
        proxy_hide_header Access-Control-Max-Age;
        {{- end }}

        {{- range hideHeaders . }}
        proxy_hide_header {{ . }};
        {{- end }}

        {{- if hasRateLimit . }}
        limit_req zone=proxera_{{ sanitizeDomain .Domain }} burst={{ rateLimitBurst . }} nodelay;
        {{- end }}
        {{- if hasLoadBalancing . }}
        proxy_pass http://proxera_{{ sanitizeDomain .Domain }};
        {{- else }}
        proxy_pass {{ .UpstreamURL }};
        {{- end }}
        proxy_http_version 1.1;
        {{- range defaultProxyHeaders . }}
        proxy_set_header {{ index . 0 }} {{ index . 1 }};
        {{- end }}

        {{- if .WebSocket }}
        # WebSocket support
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        {{- end }}

        {{- range $key, $value := mergedHeaders . }}
        proxy_set_header {{ $key }} "{{ $value }}";
        {{- end }}

        # Timeouts
        proxy_connect_timeout {{ connectTimeout . }}s;
        proxy_send_timeout {{ sendTimeout . }}s;
        proxy_read_timeout {{ readTimeout . }}s;

        {{- if hasProxyBuffering . }}
        {{- if proxyBufferingEnabled . }}
        proxy_buffering on;
        {{- if proxyBufferSize . }}
        proxy_buffer_size {{ proxyBufferSize . }};
        {{- end }}
        {{- if proxyBuffers . }}
        proxy_buffers {{ proxyBuffersCount . }} {{ proxyBuffersSize . }};
        {{- end }}
        {{- else }}
        proxy_buffering off;
        {{- end }}
        {{- end }}

        {{- if hasProxyRequestBuffering . }}
        proxy_request_buffering {{ proxyRequestBuffering . }};
        {{- end }}

        {{- if hasProxySSL . }}
        {{- if not (proxySSLVerify .) }}
        proxy_ssl_verify off;
        {{- end }}
        {{- if proxySSLServerName . }}
        proxy_ssl_server_name on;
        {{- end }}
        {{- end }}
    }
{{ end }}{{ if hasCustomNginxConfig . }}
    # Custom configuration
{{ customNginxConfig . }}
{{ end }}
    # Logging
    {{- if enableAccessLog . }}
    access_log /var/log/nginx/{{ sanitizeDomain .Domain }}_access.log proxera_metrics;
    access_log /var/log/nginx/crowdsec_access.log combined;
    {{- else }}
    access_log off;
    {{- end }}
    error_log /var/log/nginx/{{ sanitizeDomain .Domain }}_error.log;
}
`

func templateFuncMap(nginxVersion string, opts ...string) template.FuncMap {
	configDir := "/etc/nginx"
	if len(opts) > 0 && opts[0] != "" {
		configDir = opts[0]
	}
	return template.FuncMap{
		"nginxVersionAtLeast": func(minVer string) bool {
			if nginxVersion == "" {
				return false // Unknown version, use safe old syntax
			}
			return compareVersions(nginxVersion, minVer) >= 0
		},

		"sanitizeDomain": SanitizeDomain,
		"htpasswdPath": func(h types.Host) string {
			return filepath.Join(configDir, fmt.Sprintf(".htpasswd_%s", SanitizeDomain(h.Domain)))
		},

		// Server names (domain + aliases)
		"serverNames": func(h types.Host) string {
			names := []string{h.Domain}
			if h.Config != nil && len(h.Config.ServerAliases) > 0 {
				names = append(names, h.Config.ServerAliases...)
			}
			return strings.Join(names, " ")
		},

		// Redirect-only host (no proxy_pass)
		"isRedirectOnly": func(h types.Host) bool {
			return h.Config != nil && h.Config.RedirectOnly
		},

		// Load Balancing
		"hasLoadBalancing": func(h types.Host) bool {
			return h.Config != nil && h.Config.LoadBalancing != nil && len(h.Config.LoadBalancing.Servers) > 0
		},
		"lbMethod": func(h types.Host) string {
			if h.Config != nil && h.Config.LoadBalancing != nil {
				return h.Config.LoadBalancing.Method
			}
			return "round-robin"
		},
		"lbServers": func(h types.Host) []types.LoadBalancingServer {
			if h.Config != nil && h.Config.LoadBalancing != nil {
				return h.Config.LoadBalancing.Servers
			}
			return nil
		},

		// Rate Limiting
		"hasRateLimit": func(h types.Host) bool {
			return h.Config != nil && h.Config.RateLimit != nil && h.Config.RateLimit.RequestsPerSecond > 0
		},
		"rateLimitRate": func(h types.Host) int {
			if h.Config != nil && h.Config.RateLimit != nil {
				return h.Config.RateLimit.RequestsPerSecond
			}
			return 10
		},
		"rateLimitBurst": func(h types.Host) int {
			if h.Config != nil && h.Config.RateLimit != nil && h.Config.RateLimit.Burst > 0 {
				return h.Config.RateLimit.Burst
			}
			return 20
		},

		// Client Max Body Size
		"hasClientMaxBody": func(h types.Host) bool {
			return h.Config != nil && h.Config.ClientMaxBodySize != ""
		},
		"clientMaxBody": func(h types.Host) string {
			if h.Config != nil {
				return h.Config.ClientMaxBodySize
			}
			return "1m"
		},

		// IP Restrictions
		"hasIPAllowlist": func(h types.Host) bool {
			return h.Config != nil && len(h.Config.IPAllowlist) > 0
		},
		"ipAllowlist": func(h types.Host) []string {
			if h.Config != nil {
				return h.Config.IPAllowlist
			}
			return nil
		},
		"hasIPBlocklist": func(h types.Host) bool {
			return h.Config != nil && len(h.Config.IPBlocklist) > 0
		},
		"ipBlocklist": func(h types.Host) []string {
			if h.Config != nil {
				return h.Config.IPBlocklist
			}
			return nil
		},

		// Gzip
		"hasGzip": func(h types.Host) bool {
			return h.Config != nil && h.Config.Gzip != nil && h.Config.Gzip.Enabled
		},
		"isGzipOff": func(h types.Host) bool {
			// Explicit gzip off (gzip config exists but not enabled)
			return h.Config != nil && h.Config.Gzip != nil && !h.Config.Gzip.Enabled
		},
		"gzipCompLevel": func(h types.Host) int {
			if h.Config != nil && h.Config.Gzip != nil {
				return h.Config.Gzip.CompLevel
			}
			return 0
		},
		"gzipMinLength": func(h types.Host) int {
			if h.Config != nil && h.Config.Gzip != nil {
				return h.Config.Gzip.MinLength
			}
			return 0
		},
		"gzipTypes": func(h types.Host) string {
			if h.Config != nil && h.Config.Gzip != nil && len(h.Config.Gzip.Types) > 0 {
				return strings.Join(h.Config.Gzip.Types, " ")
			}
			return "text/plain text/css application/json application/javascript text/xml application/xml"
		},

		// Basic Auth - check both legacy and config
		"hasBasicAuth": func(h types.Host) bool {
			if h.Config != nil && h.Config.BasicAuth != nil && h.Config.BasicAuth.Username != "" {
				return true
			}
			return h.BasicAuth != nil && h.BasicAuth.Username != ""
		},

		// Hide Headers (proxy_hide_header)
		"hideHeaders": func(h types.Host) []string {
			defaults := []string{"X-Powered-By", "Server", "X-AspNet-Version", "X-AspNetMvc-Version"}
			disabled := make(map[string]bool)
			if h.Config != nil {
				for _, k := range h.Config.DisabledHideHeaders {
					disabled[k] = true
				}
			}
			seen := make(map[string]bool)
			result := []string{}
			for _, hdr := range defaults {
				if !disabled[hdr] && !seen[hdr] {
					seen[hdr] = true
					result = append(result, hdr)
				}
			}
			if h.Config != nil {
				for _, hdr := range h.Config.HideHeaders {
					if !seen[hdr] {
						seen[hdr] = true
						result = append(result, hdr)
					}
				}
			}
			return result
		},

		// Redirects
		"hasRedirects": func(h types.Host) bool {
			return h.Config != nil && len(h.Config.Redirects) > 0
		},
		"redirects": func(h types.Host) []types.RedirectConfig {
			if h.Config != nil {
				return h.Config.Redirects
			}
			return nil
		},

		// Custom Error Pages
		"hasCustomErrorPages": func(h types.Host) bool {
			return h.Config != nil && len(h.Config.CustomErrorPages) > 0
		},
		"customErrorPages": func(h types.Host) map[string]string {
			if h.Config != nil {
				return h.Config.CustomErrorPages
			}
			return nil
		},

		// Extra Locations
		"hasLocations": func(h types.Host) bool {
			return h.Config != nil && len(h.Config.Locations) > 0
		},
		"locations": func(h types.Host) []types.LocationConfig {
			if h.Config != nil {
				return h.Config.Locations
			}
			return nil
		},
		"locationProxyHeaders": func(h types.Host, loc types.LocationConfig) [][]string {
			defaults := [][]string{
				{"Host", "$host"},
				{"X-Real-IP", "$remote_addr"},
				{"X-Forwarded-For", "$proxy_add_x_forwarded_for"},
				{"X-Forwarded-Proto", "$scheme"},
				{"X-Forwarded-Host", "$host"},
				{"X-Forwarded-Port", "$server_port"},
			}
			var disabledKeys []string
			if loc.UseLocationProxyHeaders {
				disabledKeys = loc.DisabledProxyHeaders
			} else if h.Config != nil {
				disabledKeys = h.Config.DisabledProxyHeaders
			}
			disabled := make(map[string]bool)
			for _, k := range disabledKeys {
				disabled[k] = true
			}
			var result [][]string
			for _, d := range defaults {
				if !disabled[d[0]] {
					result = append(result, d)
				}
			}
			return result
		},
		"locationProxyPass": func(loc types.LocationConfig) string {
			if !loc.StripPrefix {
				return loc.UpstreamURL
			}
			u := loc.UpstreamURL
			if !strings.HasSuffix(u, "/") {
				u += "/"
			}
			return u
		},

		// Default proxy headers, filtered by disabled list (ordered)
		"defaultProxyHeaders": func(h types.Host) [][]string {
			defaults := [][]string{
				{"Host", "$host"},
				{"X-Real-IP", "$remote_addr"},
				{"X-Forwarded-For", "$proxy_add_x_forwarded_for"},
				{"X-Forwarded-Proto", "$scheme"},
				{"X-Forwarded-Host", "$host"},
				{"X-Forwarded-Port", "$server_port"},
			}
			disabled := make(map[string]bool)
			if h.Config != nil {
				for _, k := range h.Config.DisabledProxyHeaders {
					disabled[k] = true
				}
			}
			var result [][]string
			for _, d := range defaults {
				if !disabled[d[0]] {
					result = append(result, d)
				}
			}
			return result
		},

		// Merged headers (legacy + config), excluding default proxy headers
		"mergedHeaders": func(h types.Host) map[string]string {
			hardcoded := map[string]bool{
				"Host":              true,
				"X-Real-IP":         true,
				"X-Forwarded-For":   true,
				"X-Forwarded-Proto": true,
				"X-Forwarded-Host":  true,
				"X-Forwarded-Port":  true,
				"Upgrade":           true,
				"Connection":        true,
			}
			merged := make(map[string]string)
			for k, v := range h.Headers {
				if !hardcoded[k] {
					merged[k] = v
				}
			}
			if h.Config != nil {
				for k, v := range h.Config.Headers {
					if !hardcoded[k] {
						merged[k] = v
					}
				}
			}
			return merged
		},

		// Timeouts
		"connectTimeout": func(h types.Host) int {
			if h.Config != nil && h.Config.Timeouts != nil && h.Config.Timeouts.Connect > 0 {
				return h.Config.Timeouts.Connect
			}
			return 60
		},
		"sendTimeout": func(h types.Host) int {
			if h.Config != nil && h.Config.Timeouts != nil && h.Config.Timeouts.Send > 0 {
				return h.Config.Timeouts.Send
			}
			return 60
		},
		"readTimeout": func(h types.Host) int {
			if h.Config != nil && h.Config.Timeouts != nil && h.Config.Timeouts.Read > 0 {
				return h.Config.Timeouts.Read
			}
			return 60
		},

		// Proxy Buffering
		"hasProxyBuffering": func(h types.Host) bool {
			return h.Config != nil && h.Config.ProxyBuffering != nil
		},
		"proxyBufferingEnabled": func(h types.Host) bool {
			return h.Config != nil && h.Config.ProxyBuffering != nil && h.Config.ProxyBuffering.Enabled
		},
		"proxyBufferSize": func(h types.Host) string {
			if h.Config != nil && h.Config.ProxyBuffering != nil {
				return h.Config.ProxyBuffering.BufferSize
			}
			return ""
		},
		"proxyBuffers": func(h types.Host) bool {
			return h.Config != nil && h.Config.ProxyBuffering != nil && h.Config.ProxyBuffering.BuffersCount > 0
		},
		"proxyBuffersCount": func(h types.Host) int {
			if h.Config != nil && h.Config.ProxyBuffering != nil {
				return h.Config.ProxyBuffering.BuffersCount
			}
			return 8
		},
		"proxyBuffersSize": func(h types.Host) string {
			if h.Config != nil && h.Config.ProxyBuffering != nil && h.Config.ProxyBuffering.BuffersSize != "" {
				return h.Config.ProxyBuffering.BuffersSize
			}
			return "8k"
		},

		// Proxy Request Buffering
		"hasProxyRequestBuffering": func(h types.Host) bool {
			return h.Config != nil && h.Config.ProxyRequestBuffering != nil
		},
		"proxyRequestBuffering": func(h types.Host) string {
			if h.Config != nil && h.Config.ProxyRequestBuffering != nil {
				if *h.Config.ProxyRequestBuffering {
					return "on"
				}
				return "off"
			}
			return "on"
		},

		// Proxy SSL
		"hasProxySSL": func(h types.Host) bool {
			return h.Config != nil && h.Config.ProxySSL != nil
		},
		"proxySSLVerify": func(h types.Host) bool {
			if h.Config != nil && h.Config.ProxySSL != nil {
				return h.Config.ProxySSL.Verify
			}
			return true
		},
		"proxySSLServerName": func(h types.Host) bool {
			return h.Config != nil && h.Config.ProxySSL != nil && h.Config.ProxySSL.ServerName
		},

		// CORS
		"hasCORS": func(h types.Host) bool {
			return h.Config != nil && h.Config.CORS != nil && h.Config.CORS.Enabled
		},
		"corsDynamic": func(h types.Host) bool {
			return h.Config != nil && h.Config.CORS != nil && h.Config.CORS.Dynamic
		},
		"corsOrigins": func(h types.Host) []string {
			if h.Config != nil && h.Config.CORS != nil && len(h.Config.CORS.AllowedOrigins) > 0 {
				return h.Config.CORS.AllowedOrigins
			}
			return []string{"*"}
		},
		"corsMethods": func(h types.Host) []string {
			if h.Config != nil && h.Config.CORS != nil && len(h.Config.CORS.AllowedMethods) > 0 {
				return h.Config.CORS.AllowedMethods
			}
			return []string{"GET", "POST", "OPTIONS"}
		},
		"corsHeaders": func(h types.Host) []string {
			if h.Config != nil && h.Config.CORS != nil && len(h.Config.CORS.AllowedHeaders) > 0 {
				return h.Config.CORS.AllowedHeaders
			}
			return []string{"*"}
		},
		"corsCredentials": func(h types.Host) bool {
			return h.Config != nil && h.Config.CORS != nil && h.Config.CORS.AllowCredentials
		},
		"corsMaxAge": func(h types.Host) int {
			if h.Config != nil && h.Config.CORS != nil {
				return h.Config.CORS.MaxAge
			}
			return 0
		},
		"joinStrings": func(strs []string, sep string) string {
			return strings.Join(strs, sep)
		},

		// HSTS
		"hasHSTS": func(h types.Host) bool {
			return h.Config != nil && h.Config.HSTS != nil && h.Config.HSTS.Enabled
		},
		"hstsMaxAge": func(h types.Host) int {
			if h.Config != nil && h.Config.HSTS != nil && h.Config.HSTS.MaxAge > 0 {
				return h.Config.HSTS.MaxAge
			}
			return 31536000
		},
		"hstsIncludeSubDomains": func(h types.Host) bool {
			return h.Config != nil && h.Config.HSTS != nil && h.Config.HSTS.IncludeSubDomains
		},
		"hstsPreload": func(h types.Host) bool {
			return h.Config != nil && h.Config.HSTS != nil && h.Config.HSTS.Preload
		},

		// Security Headers
		"hasCSP": func(h types.Host) bool {
			return h.Config != nil && h.Config.SecurityHeaders != nil && h.Config.SecurityHeaders.CSP != ""
		},
		"csp": func(h types.Host) string {
			if h.Config != nil && h.Config.SecurityHeaders != nil {
				return h.Config.SecurityHeaders.CSP
			}
			return ""
		},
		"hasReferrerPolicy": func(h types.Host) bool {
			return h.Config != nil && h.Config.SecurityHeaders != nil && h.Config.SecurityHeaders.ReferrerPolicy != ""
		},
		"referrerPolicy": func(h types.Host) string {
			if h.Config != nil && h.Config.SecurityHeaders != nil {
				return h.Config.SecurityHeaders.ReferrerPolicy
			}
			return ""
		},
		"hasPermissionsPolicy": func(h types.Host) bool {
			return h.Config != nil && h.Config.SecurityHeaders != nil && h.Config.SecurityHeaders.PermissionsPolicy != ""
		},
		"permissionsPolicy": func(h types.Host) string {
			if h.Config != nil && h.Config.SecurityHeaders != nil {
				return h.Config.SecurityHeaders.PermissionsPolicy
			}
			return ""
		},
		"xFrameOptions": func(h types.Host) string {
			if h.Config != nil && h.Config.SecurityHeaders != nil && h.Config.SecurityHeaders.XFrameOptions != "" {
				return h.Config.SecurityHeaders.XFrameOptions
			}
			return "SAMEORIGIN"
		},

		// Custom Nginx Config
		"hasCustomNginxConfig": func(h types.Host) bool {
			return h.Config != nil && h.Config.CustomNginxConfig != ""
		},
		"customNginxConfig": func(h types.Host) string {
			if h.Config != nil {
				// Indent each line with 4 spaces for proper nginx formatting
				lines := strings.Split(h.Config.CustomNginxConfig, "\n")
				for i, line := range lines {
					if line != "" {
						lines[i] = "    " + line
					}
				}
				return strings.Join(lines, "\n")
			}
			return ""
		},

		// HTTP/2
		"enableHTTP2": func(h types.Host) bool {
			if h.Config != nil && h.Config.HTTP2 != nil {
				return *h.Config.HTTP2
			}
			return true // default: enabled
		},

		// Access Log
		"enableAccessLog": func(h types.Host) bool {
			if h.Config != nil && h.Config.AccessLog != nil {
				return *h.Config.AccessLog
			}
			return true // default: enabled
		},

		// HTTP Proxy (dual-mode: proxy on both HTTP and HTTPS instead of redirect)
		"httpProxy": func(h types.Host) bool {
			return h.Config != nil && h.Config.HTTPProxy != nil && *h.Config.HTTPProxy
		},

		// Trusted Proxies — emit set_real_ip_from directives so nginx uses the appropriate
		// header to determine the real client IP instead of the upstream proxy's IP.
		"hasTrustedProxies": func(h types.Host) bool {
			if h.Config == nil {
				return false
			}
			return len(h.Config.TrustedProxies) > 0 || h.Config.CloudflareRealIP
		},
		"trustedProxies": func(h types.Host) []string {
			if h.Config == nil {
				return nil
			}
			var proxies []string
			if h.Config.CloudflareRealIP {
				proxies = append(proxies, cloudflareIPRanges...)
			}
			proxies = append(proxies, h.Config.TrustedProxies...)
			return proxies
		},
		"realIPHeader": func(h types.Host) string {
			if h.Config != nil && h.Config.CloudflareRealIP {
				return "CF-Connecting-IP"
			}
			return "X-Forwarded-For"
		},

		// Default Security Headers (X-Frame-Options, X-Content-Type-Options)
		// Only output when security_headers config is present
		"hasDefaultSecurityHeaders": func(h types.Host) bool {
			return h.Config != nil && h.Config.SecurityHeaders != nil
		},
	}
}

// GenerateConfig creates nginx configuration for a host
func GenerateConfig(host types.Host, configPath string) error {
	// Ensure upstream URL has a scheme before validation (nginx proxy_pass requires it)
	if host.UpstreamURL != "" &&
		!strings.HasPrefix(host.UpstreamURL, "http://") &&
		!strings.HasPrefix(host.UpstreamURL, "https://") {
		host.UpstreamURL = "http://" + host.UpstreamURL
	}

	// Validate all user-controlled fields to prevent nginx directive injection
	if err := ValidateHost(&host); err != nil {
		return fmt.Errorf("config validation failed: %w", err)
	}

	// Detect nginx version for version-aware template directives
	nginxVersion := DetectNginxVersion()

	// Create template with custom functions
	tmpl, err := template.New("nginx").Funcs(templateFuncMap(nginxVersion, configPath)).Parse(nginxConfigTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse template: %w", err)
	}

	// Sanitize domain for filename
	safeDomain := SanitizeDomain(host.Domain)
	filename := filepath.Join(configPath, fmt.Sprintf("proxera_%s.conf", safeDomain))

	// Create config file
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create config file: %w", err)
	}
	defer file.Close()

	// Execute template
	if err := tmpl.Execute(file, host); err != nil {
		return fmt.Errorf("failed to generate config: %w", err)
	}

	log.Printf("Generated config: %s", filename)
	return nil
}

// EnableConfig enables a host config. When configPath == enabledPath (conf.d mode),
// this is a no-op because GenerateConfig already writes directly to the active directory.
// In symlink mode (sites-available/sites-enabled), it creates a symlink.
func EnableConfig(domain, configPath, enabledPath string) error {
	// conf.d mode: config is already in the active directory, nothing to do
	if configPath == enabledPath {
		return nil
	}

	// Symlink mode (legacy sites-available/sites-enabled)
	safeDomain := SanitizeDomain(domain)
	source := filepath.Join(configPath, fmt.Sprintf("proxera_%s.conf", safeDomain))
	target := filepath.Join(enabledPath, fmt.Sprintf("proxera_%s.conf", safeDomain))

	// Remove existing symlink if present
	os.Remove(target)

	// Create symlink
	if err := os.Symlink(source, target); err != nil {
		return fmt.Errorf("failed to enable config: %w", err)
	}

	log.Printf("Enabled config for %s", domain)
	return nil
}

// DisableConfig removes a host config from the active directory.
// In conf.d mode, this removes the actual config file.
// In symlink mode, this removes the symlink from sites-enabled.
func DisableConfig(domain, enabledPath string) error {
	safeDomain := SanitizeDomain(domain)
	target := filepath.Join(enabledPath, fmt.Sprintf("proxera_%s.conf", safeDomain))

	if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to disable config: %w", err)
	}

	log.Printf("Disabled config for %s", domain)
	return nil
}
