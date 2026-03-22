package nginx

import (
	"strings"
	"testing"

	"github.com/proxera/agent/pkg/types"
)

// --- ValidateHost tests ---

func TestValidateHost_ValidMinimal(t *testing.T) {
	host := &types.Host{
		Domain:      "example.com",
		UpstreamURL: "http://127.0.0.1:8080",
	}
	if err := ValidateHost(host); err != nil {
		t.Fatalf("expected nil error for valid minimal host, got: %v", err)
	}
}

func TestValidateHost_ValidMinimal_DomainOnly(t *testing.T) {
	host := &types.Host{
		Domain: "example.com",
	}
	if err := ValidateHost(host); err != nil {
		t.Fatalf("expected nil error for host with domain only, got: %v", err)
	}
}

func TestValidateHost_EmptyDomain(t *testing.T) {
	host := &types.Host{
		Domain:      "",
		UpstreamURL: "http://127.0.0.1:8080",
	}
	err := ValidateHost(host)
	if err == nil {
		t.Fatal("expected error for empty domain, got nil")
	}
	if !strings.Contains(err.Error(), "domain is required") {
		t.Fatalf("expected 'domain is required' error, got: %v", err)
	}
}

func TestValidateHost_InvalidDomain(t *testing.T) {
	cases := []struct {
		name   string
		domain string
	}{
		{"spaces", "exam ple.com"},
		{"special chars", "exam!ple.com"},
		{"underscore", "exam_ple.com"},
		{"trailing dot", "example.com."},
		{"double dot", "example..com"},
		{"starts with hyphen", "-example.com"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			host := &types.Host{
				Domain:      tc.domain,
				UpstreamURL: "http://127.0.0.1:8080",
			}
			err := ValidateHost(host)
			if err == nil {
				t.Fatalf("expected error for invalid domain %q, got nil", tc.domain)
			}
			if !strings.Contains(err.Error(), "invalid domain format") {
				t.Fatalf("expected 'invalid domain format' error, got: %v", err)
			}
		})
	}
}

func TestValidateHost_WildcardDomain(t *testing.T) {
	host := &types.Host{
		Domain:      "*.example.com",
		UpstreamURL: "http://127.0.0.1:8080",
	}
	if err := ValidateHost(host); err != nil {
		t.Fatalf("expected nil error for wildcard domain, got: %v", err)
	}
}

func TestValidateHost_InvalidUpstreamURL(t *testing.T) {
	cases := []struct {
		name string
		url  string
	}{
		{"no scheme or host", "not-a-url"},
		{"missing host", "http://"},
		{"just path", "/path/to/thing"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			host := &types.Host{
				Domain:      "example.com",
				UpstreamURL: tc.url,
			}
			err := ValidateHost(host)
			if err == nil {
				t.Fatalf("expected error for invalid upstream URL %q, got nil", tc.url)
			}
			if !strings.Contains(err.Error(), "invalid upstream URL") {
				t.Fatalf("expected 'invalid upstream URL' error, got: %v", err)
			}
		})
	}
}

func TestValidateHost_ValidIPAllowlist(t *testing.T) {
	host := &types.Host{
		Domain:      "example.com",
		UpstreamURL: "http://127.0.0.1:8080",
		Config: &types.AdvancedConfig{
			IPAllowlist: []string{
				"192.168.1.1",
				"10.0.0.0/8",
				"172.16.0.0/12",
				"::1",
				"2001:db8::/32",
			},
		},
	}
	if err := ValidateHost(host); err != nil {
		t.Fatalf("expected nil error for valid IP allowlist, got: %v", err)
	}
}

func TestValidateHost_InvalidIPAllowlist(t *testing.T) {
	cases := []struct {
		name string
		ip   string
	}{
		{"out of range octets", "999.999.999.999"},
		{"incomplete", "192.168.1"},
		{"text", "not-an-ip"},
		{"invalid CIDR", "10.0.0.0/33"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			host := &types.Host{
				Domain:      "example.com",
				UpstreamURL: "http://127.0.0.1:8080",
				Config: &types.AdvancedConfig{
					IPAllowlist: []string{tc.ip},
				},
			}
			err := ValidateHost(host)
			if err == nil {
				t.Fatalf("expected error for invalid IP %q, got nil", tc.ip)
			}
			if !strings.Contains(err.Error(), "invalid IP allowlist entry") {
				t.Fatalf("expected 'invalid IP allowlist entry' error, got: %v", err)
			}
		})
	}
}

func TestValidateHost_ValidRedirects(t *testing.T) {
	host := &types.Host{
		Domain:      "example.com",
		UpstreamURL: "http://127.0.0.1:8080",
		Config: &types.AdvancedConfig{
			Redirects: []types.RedirectConfig{
				{Source: "/old-path", Target: "https://example.com/new-path", Code: 301},
				{Source: "/temp", Target: "https://example.com/other", Code: 302},
				{Source: "/see-other", Target: "https://example.com/dest", Code: 303},
				{Source: "/preserve-method", Target: "https://example.com/dest2", Code: 307},
				{Source: "/permanent-method", Target: "https://example.com/dest3", Code: 308},
			},
		},
	}
	if err := ValidateHost(host); err != nil {
		t.Fatalf("expected nil error for valid redirects, got: %v", err)
	}
}

func TestValidateHost_InvalidRedirectCode(t *testing.T) {
	host := &types.Host{
		Domain:      "example.com",
		UpstreamURL: "http://127.0.0.1:8080",
		Config: &types.AdvancedConfig{
			Redirects: []types.RedirectConfig{
				{Source: "/path", Target: "https://example.com/dest", Code: 200},
			},
		},
	}
	err := ValidateHost(host)
	if err == nil {
		t.Fatal("expected error for redirect code 200, got nil")
	}
	if !strings.Contains(err.Error(), "invalid redirect code") {
		t.Fatalf("expected 'invalid redirect code' error, got: %v", err)
	}
}

func TestValidateHost_NginxInjectionInRedirect(t *testing.T) {
	cases := []struct {
		name   string
		target string
	}{
		{"semicolon", "https://example.com/path;injection"},
		{"newline", "https://example.com/path\ninjection"},
		{"carriage return", "https://example.com/path\rinjection"},
		{"open brace", "https://example.com/path{injection"},
		{"close brace", "https://example.com/path}injection"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			host := &types.Host{
				Domain:      "example.com",
				UpstreamURL: "http://127.0.0.1:8080",
				Config: &types.AdvancedConfig{
					Redirects: []types.RedirectConfig{
						{Source: "/path", Target: tc.target, Code: 301},
					},
				},
			}
			err := ValidateHost(host)
			if err == nil {
				t.Fatalf("expected error for injection in redirect target %q, got nil", tc.target)
			}
			if !strings.Contains(err.Error(), "invalid redirect target") {
				t.Fatalf("expected 'invalid redirect target' error, got: %v", err)
			}
		})
	}
}

func TestValidateHost_DangerousCustomConfig(t *testing.T) {
	// Test every dangerous directive in the expanded blocklist
	dangerousDirectives := []struct {
		name    string
		content string
	}{
		{"lua_", "lua_code_cache on;"},
		{"load_module", "load_module modules/ngx_http_perl.so;"},
		{"include", "include /etc/passwd;"},
		{"ssl_certificate", "ssl_certificate /etc/ssl/certs/fake.pem;"},
		{"root", "root /var/www/html;"},
		{"alias", "alias /etc/;"},
		{"proxy_pass", "proxy_pass http://evil.com;"},
		{"upstream", "upstream backend { server 127.0.0.1:9090; }"},
		{"server", "server { listen 80; }"},
		{"listen", "listen 8080;"},
		{"resolver", "resolver 8.8.8.8;"},
		{"perl", "perl_set $var 'sub { return 1; }';"},
		{"js_", "js_import http from http.js;"},
		{"wasm_", "wasm_module ngx_http_wasm;"},
		{"env", "env SECRET_KEY;"},
		{"error_log", "error_log /dev/null;"},
		{"set $", "set $var 'malicious';"},
		{"rewrite_log", "rewrite_log on;"},
	}
	for _, tc := range dangerousDirectives {
		t.Run(tc.name, func(t *testing.T) {
			host := &types.Host{
				Domain:      "example.com",
				UpstreamURL: "http://127.0.0.1:8080",
				Config: &types.AdvancedConfig{
					CustomNginxConfig: tc.content,
				},
			}
			err := ValidateHost(host)
			if err == nil {
				t.Fatalf("expected error for dangerous directive %q, got nil", tc.name)
			}
			if !strings.Contains(err.Error(), "disallowed directive") {
				t.Fatalf("expected 'disallowed directive' error, got: %v", err)
			}
		})
	}
}

func TestValidateHost_DangerousCustomConfigCaseInsensitive(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{"LOAD_MODULE uppercase", "LOAD_MODULE modules/ngx_http_perl.so;"},
		{"Load_Module mixed", "Load_Module modules/ngx_http_perl.so;"},
		{"LUA_ uppercase", "LUA_CODE_CACHE on;"},
		{"INCLUDE uppercase", "INCLUDE /etc/passwd;"},
		{"ROOT uppercase", "ROOT /var/www/html;"},
		{"PROXY_PASS uppercase", "PROXY_PASS http://evil.com;"},
		{"UPSTREAM uppercase", "UPSTREAM backend { server 127.0.0.1:9090; }"},
		{"LISTEN uppercase", "LISTEN 8080;"},
		{"PERL uppercase", "PERL_SET $var 'sub { return 1; }';"},
		{"SET $ mixed case", "SET $var 'malicious';"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			host := &types.Host{
				Domain:      "example.com",
				UpstreamURL: "http://127.0.0.1:8080",
				Config: &types.AdvancedConfig{
					CustomNginxConfig: tc.content,
				},
			}
			err := ValidateHost(host)
			if err == nil {
				t.Fatalf("expected error for case-insensitive dangerous directive %q, got nil", tc.name)
			}
			if !strings.Contains(err.Error(), "disallowed directive") {
				t.Fatalf("expected 'disallowed directive' error, got: %v", err)
			}
		})
	}
}

func TestValidateHost_SafeCustomConfig(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{"proxy_read_timeout", "proxy_read_timeout 30s;"},
		{"proxy_connect_timeout", "proxy_connect_timeout 60s;"},
		{"proxy_send_timeout", "proxy_send_timeout 60s;"},
		{"add_header", "add_header X-Custom-Header value;"},
		{"proxy_set_header", "proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;"},
		{"client_body_timeout", "client_body_timeout 60s;"},
		{"keepalive_timeout", "keepalive_timeout 65;"},
		{"proxy_cache_valid", "proxy_cache_valid 200 1d;"},
		{"multiline safe config", "proxy_read_timeout 30s;\nproxy_connect_timeout 60s;\nadd_header X-Custom value;"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			host := &types.Host{
				Domain:      "example.com",
				UpstreamURL: "http://127.0.0.1:8080",
				Config: &types.AdvancedConfig{
					CustomNginxConfig: tc.content,
				},
			}
			if err := ValidateHost(host); err != nil {
				t.Fatalf("expected nil error for safe custom config %q, got: %v", tc.name, err)
			}
		})
	}
}

func TestValidateHost_CustomConfigLengthLimit(t *testing.T) {
	// 16 KB = 16384 bytes; create a string just over the limit
	oversize := strings.Repeat("a", 16385)
	host := &types.Host{
		Domain:      "example.com",
		UpstreamURL: "http://127.0.0.1:8080",
		Config: &types.AdvancedConfig{
			CustomNginxConfig: oversize,
		},
	}
	err := ValidateHost(host)
	if err == nil {
		t.Fatal("expected error for custom config exceeding 16 KB, got nil")
	}
	if !strings.Contains(err.Error(), "exceeds maximum length") {
		t.Fatalf("expected 'exceeds maximum length' error, got: %v", err)
	}

	// Verify that exactly 16384 bytes is accepted
	atLimit := strings.Repeat("a", 16384)
	hostOk := &types.Host{
		Domain:      "example.com",
		UpstreamURL: "http://127.0.0.1:8080",
		Config: &types.AdvancedConfig{
			CustomNginxConfig: atLimit,
		},
	}
	if err := ValidateHost(hostOk); err != nil {
		t.Fatalf("expected nil error for custom config at exactly 16384 bytes, got: %v", err)
	}
}

func TestValidateHost_CustomConfigLineLimit(t *testing.T) {
	// 201 lines = 200 newlines + last line
	lines := make([]string, 201)
	for i := range lines {
		lines[i] = "add_header X-Test value"
	}
	overLines := strings.Join(lines, "\n")

	host := &types.Host{
		Domain:      "example.com",
		UpstreamURL: "http://127.0.0.1:8080",
		Config: &types.AdvancedConfig{
			CustomNginxConfig: overLines,
		},
	}
	err := ValidateHost(host)
	if err == nil {
		t.Fatal("expected error for custom config exceeding 200 lines, got nil")
	}
	if !strings.Contains(err.Error(), "exceeds maximum of") {
		t.Fatalf("expected 'exceeds maximum of' error, got: %v", err)
	}

	// Verify that exactly 200 lines is accepted
	linesOk := make([]string, 200)
	for i := range linesOk {
		linesOk[i] = "add_header X-Test value"
	}
	atLimit := strings.Join(linesOk, "\n")

	hostOk := &types.Host{
		Domain:      "example.com",
		UpstreamURL: "http://127.0.0.1:8080",
		Config: &types.AdvancedConfig{
			CustomNginxConfig: atLimit,
		},
	}
	if err := ValidateHost(hostOk); err != nil {
		t.Fatalf("expected nil error for custom config at exactly 200 lines, got: %v", err)
	}
}

func TestValidateHost_ValidLoadBalancing(t *testing.T) {
	host := &types.Host{
		Domain:      "example.com",
		UpstreamURL: "http://127.0.0.1:8080",
		Config: &types.AdvancedConfig{
			LoadBalancing: &types.LoadBalancingConfig{
				Method: "least-conn",
				Servers: []types.LoadBalancingServer{
					{Address: "192.168.1.10:8080", Weight: 5},
					{Address: "192.168.1.11:8080", Weight: 3},
				},
			},
		},
	}
	if err := ValidateHost(host); err != nil {
		t.Fatalf("expected nil error for valid load balancing config, got: %v", err)
	}
}

func TestValidateHost_ValidLoadBalancing_AllMethods(t *testing.T) {
	methods := []string{"round-robin", "least-conn", "ip-hash"}
	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			host := &types.Host{
				Domain:      "example.com",
				UpstreamURL: "http://127.0.0.1:8080",
				Config: &types.AdvancedConfig{
					LoadBalancing: &types.LoadBalancingConfig{
						Method: method,
						Servers: []types.LoadBalancingServer{
							{Address: "10.0.0.1:80"},
						},
					},
				},
			}
			if err := ValidateHost(host); err != nil {
				t.Fatalf("expected nil error for LB method %q, got: %v", method, err)
			}
		})
	}
}

func TestValidateHost_InvalidLBMethod(t *testing.T) {
	host := &types.Host{
		Domain:      "example.com",
		UpstreamURL: "http://127.0.0.1:8080",
		Config: &types.AdvancedConfig{
			LoadBalancing: &types.LoadBalancingConfig{
				Method: "random",
				Servers: []types.LoadBalancingServer{
					{Address: "192.168.1.10:8080"},
				},
			},
		},
	}
	err := ValidateHost(host)
	if err == nil {
		t.Fatal("expected error for invalid LB method, got nil")
	}
	if !strings.Contains(err.Error(), "invalid load balancing method") {
		t.Fatalf("expected 'invalid load balancing method' error, got: %v", err)
	}
}

func TestValidateHost_ValidCORS(t *testing.T) {
	host := &types.Host{
		Domain:      "example.com",
		UpstreamURL: "http://127.0.0.1:8080",
		Config: &types.AdvancedConfig{
			CORS: &types.CORSConfig{
				Enabled:          true,
				AllowedOrigins:   []string{"https://app.example.com", "https://admin.example.com", "*"},
				AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
				AllowedHeaders:   []string{"Authorization", "Content-Type", "*"},
				AllowCredentials: true,
				MaxAge:           3600,
			},
		},
	}
	if err := ValidateHost(host); err != nil {
		t.Fatalf("expected nil error for valid CORS config, got: %v", err)
	}
}

func TestValidateHost_InvalidCORSOrigin(t *testing.T) {
	cases := []struct {
		name   string
		origin string
		errMsg string
	}{
		{"semicolon injection", "https://example.com;evil", "invalid CORS origin"},
		{"newline injection", "https://example.com\nevil", "invalid CORS origin"},
		{"brace injection", "https://example.com{evil}", "invalid CORS origin"},
		{"no host", "not-a-url", "invalid CORS origin URL"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			host := &types.Host{
				Domain:      "example.com",
				UpstreamURL: "http://127.0.0.1:8080",
				Config: &types.AdvancedConfig{
					CORS: &types.CORSConfig{
						Enabled:        true,
						AllowedOrigins: []string{tc.origin},
					},
				},
			}
			err := ValidateHost(host)
			if err == nil {
				t.Fatalf("expected error for invalid CORS origin %q, got nil", tc.origin)
			}
			if !strings.Contains(err.Error(), tc.errMsg) {
				t.Fatalf("expected error containing %q, got: %v", tc.errMsg, err)
			}
		})
	}
}

// --- containsNginxInjection tests ---

func TestContainsNginxInjection_Semicolon(t *testing.T) {
	if !containsNginxInjection("hello;world") {
		t.Fatal("expected true for string containing semicolon")
	}
}

func TestContainsNginxInjection_Newline(t *testing.T) {
	if !containsNginxInjection("hello\nworld") {
		t.Fatal("expected true for string containing newline")
	}
}

func TestContainsNginxInjection_CarriageReturn(t *testing.T) {
	if !containsNginxInjection("hello\rworld") {
		t.Fatal("expected true for string containing carriage return")
	}
}

func TestContainsNginxInjection_OpenBrace(t *testing.T) {
	if !containsNginxInjection("hello{world") {
		t.Fatal("expected true for string containing open brace")
	}
}

func TestContainsNginxInjection_CloseBrace(t *testing.T) {
	if !containsNginxInjection("hello}world") {
		t.Fatal("expected true for string containing close brace")
	}
}

func TestContainsNginxInjection_Clean(t *testing.T) {
	if containsNginxInjection("hello-world_test.value") {
		t.Fatal("expected false for clean string")
	}
}

func TestContainsNginxInjection_Empty(t *testing.T) {
	if containsNginxInjection("") {
		t.Fatal("expected false for empty string")
	}
}

// --- Additional edge case tests for completeness ---

func TestValidateHost_NilConfig(t *testing.T) {
	host := &types.Host{
		Domain:      "example.com",
		UpstreamURL: "http://127.0.0.1:8080",
		Config:      nil,
	}
	if err := ValidateHost(host); err != nil {
		t.Fatalf("expected nil error for host with nil config, got: %v", err)
	}
}

func TestValidateHost_InvalidRedirectSource(t *testing.T) {
	cases := []struct {
		name   string
		source string
	}{
		{"empty source", ""},
		{"no leading slash", "old-path"},
		{"injection in source", "/path;evil"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			host := &types.Host{
				Domain:      "example.com",
				UpstreamURL: "http://127.0.0.1:8080",
				Config: &types.AdvancedConfig{
					Redirects: []types.RedirectConfig{
						{Source: tc.source, Target: "https://example.com/dest", Code: 301},
					},
				},
			}
			err := ValidateHost(host)
			if err == nil {
				t.Fatalf("expected error for invalid redirect source %q, got nil", tc.source)
			}
			if !strings.Contains(err.Error(), "invalid redirect source") {
				t.Fatalf("expected 'invalid redirect source' error, got: %v", err)
			}
		})
	}
}

func TestValidateHost_InvalidLBServerAddress(t *testing.T) {
	host := &types.Host{
		Domain:      "example.com",
		UpstreamURL: "http://127.0.0.1:8080",
		Config: &types.AdvancedConfig{
			LoadBalancing: &types.LoadBalancingConfig{
				Method: "round-robin",
				Servers: []types.LoadBalancingServer{
					{Address: "not a valid address!"},
				},
			},
		},
	}
	err := ValidateHost(host)
	if err == nil {
		t.Fatal("expected error for invalid LB server address, got nil")
	}
	if !strings.Contains(err.Error(), "invalid load balancing server address") {
		t.Fatalf("expected 'invalid load balancing server address' error, got: %v", err)
	}
}

func TestValidateHost_LBServerAddressWithInjection(t *testing.T) {
	host := &types.Host{
		Domain:      "example.com",
		UpstreamURL: "http://127.0.0.1:8080",
		Config: &types.AdvancedConfig{
			LoadBalancing: &types.LoadBalancingConfig{
				Method: "round-robin",
				Servers: []types.LoadBalancingServer{
					{Address: "192.168.1.1:8080;evil"},
				},
			},
		},
	}
	err := ValidateHost(host)
	if err == nil {
		t.Fatal("expected error for LB server address with injection, got nil")
	}
	if !strings.Contains(err.Error(), "invalid load balancing server address") {
		t.Fatalf("expected 'invalid load balancing server address' error, got: %v", err)
	}
}

func TestValidateHost_InvalidCORSMethod(t *testing.T) {
	host := &types.Host{
		Domain:      "example.com",
		UpstreamURL: "http://127.0.0.1:8080",
		Config: &types.AdvancedConfig{
			CORS: &types.CORSConfig{
				Enabled:        true,
				AllowedOrigins: []string{"https://example.com"},
				AllowedMethods: []string{"INVALID_METHOD"},
			},
		},
	}
	err := ValidateHost(host)
	if err == nil {
		t.Fatal("expected error for invalid CORS method, got nil")
	}
	if !strings.Contains(err.Error(), "invalid CORS method") {
		t.Fatalf("expected 'invalid CORS method' error, got: %v", err)
	}
}

func TestValidateHost_CORSHeaderInjection(t *testing.T) {
	host := &types.Host{
		Domain:      "example.com",
		UpstreamURL: "http://127.0.0.1:8080",
		Config: &types.AdvancedConfig{
			CORS: &types.CORSConfig{
				Enabled:        true,
				AllowedOrigins: []string{"https://example.com"},
				AllowedHeaders: []string{"Authorization;evil"},
			},
		},
	}
	err := ValidateHost(host)
	if err == nil {
		t.Fatal("expected error for CORS header injection, got nil")
	}
	if !strings.Contains(err.Error(), "invalid CORS header") {
		t.Fatalf("expected 'invalid CORS header' error, got: %v", err)
	}
}

func TestValidateHost_ValidServerAlias(t *testing.T) {
	host := &types.Host{
		Domain:      "example.com",
		UpstreamURL: "http://127.0.0.1:8080",
		Config: &types.AdvancedConfig{
			ServerAliases: []string{"www.example.com", "api.example.com"},
		},
	}
	if err := ValidateHost(host); err != nil {
		t.Fatalf("expected nil error for valid server aliases, got: %v", err)
	}
}

func TestValidateHost_InvalidServerAlias(t *testing.T) {
	host := &types.Host{
		Domain:      "example.com",
		UpstreamURL: "http://127.0.0.1:8080",
		Config: &types.AdvancedConfig{
			ServerAliases: []string{"invalid domain!"},
		},
	}
	err := ValidateHost(host)
	if err == nil {
		t.Fatal("expected error for invalid server alias, got nil")
	}
	if !strings.Contains(err.Error(), "invalid server alias") {
		t.Fatalf("expected 'invalid server alias' error, got: %v", err)
	}
}

func TestValidateHost_CustomHeaderInjection(t *testing.T) {
	host := &types.Host{
		Domain:      "example.com",
		UpstreamURL: "http://127.0.0.1:8080",
		Config: &types.AdvancedConfig{
			Headers: map[string]string{
				"X-Injected": "value;evil",
			},
		},
	}
	err := ValidateHost(host)
	if err == nil {
		t.Fatal("expected error for custom header injection, got nil")
	}
	if !strings.Contains(err.Error(), "invalid custom header") {
		t.Fatalf("expected 'invalid custom header' error, got: %v", err)
	}
}
