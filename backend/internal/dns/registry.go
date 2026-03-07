package dns

import "fmt"

var registry = map[string]Provider{
	"cloudflare": &CloudflareProvider{},
	"ionos":      &IONOSProvider{},
	"porkbun":    &PorkbunProvider{},
}

// Get returns the Provider implementation for the given provider name.
func Get(name string) (Provider, error) {
	p, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("unsupported DNS provider: %q", name)
	}
	return p, nil
}
