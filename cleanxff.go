// Package cleanxff is a Traefik middleware plugin that removes trusted IP
// addresses from the X-Forwarded-For header before the request is forwarded
// to the upstream backend.
//
// Typical use case: you have one or more trusted proxies in front of Traefik
// (load balancer, CDN, ingress). Traefik itself uses `forwardedHeaders.trustedIPs`
// on the entryPoint to determine the real client IP, but the trusted proxy
// addresses remain in the X-Forwarded-For chain and are forwarded to the
// backend. This plugin strips them, leaving only the real client IP
// (and any untrusted intermediate hops, if they exist).
package cleanxff

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
)

// Config is the plugin configuration.
type Config struct {
	// TrustedCIDRs is a list of CIDR ranges that are considered trusted
	// proxies. Any IP from these ranges will be removed from the XFF header.
	// Required. Examples: "10.0.0.0/8", "173.245.48.0/20", "2001:db8::/32".
	TrustedCIDRs []string `json:"trustedCIDRs,omitempty"`

	// HeaderName is the name of the header to clean. Defaults to
	// "X-Forwarded-For". Override only if you use a non-standard header.
	HeaderName string `json:"headerName,omitempty"`
}

// CreateConfig returns a Config with sane defaults.
func CreateConfig() *Config {
	return &Config{
		HeaderName: "X-Forwarded-For",
	}
}

// CleanXFF is the middleware handler.
type CleanXFF struct {
	next       http.Handler
	name       string
	trusted    []*net.IPNet
	headerName string
}

// New creates a new CleanXFF middleware instance.
func New(ctx context.Context, next http.Handler, cfg *Config, name string) (http.Handler, error) {
	_ = ctx

	if cfg == nil {
		return nil, fmt.Errorf("cleanxff: config is nil")
	}
	if len(cfg.TrustedCIDRs) == 0 {
		return nil, fmt.Errorf("cleanxff: trustedCIDRs must not be empty")
	}

	nets := make([]*net.IPNet, 0, len(cfg.TrustedCIDRs))
	for _, c := range cfg.TrustedCIDRs {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		_, n, err := net.ParseCIDR(c)
		if err != nil {
			return nil, fmt.Errorf("cleanxff: invalid CIDR %q: %w", c, err)
		}
		nets = append(nets, n)
	}
	if len(nets) == 0 {
		return nil, fmt.Errorf("cleanxff: no valid CIDRs provided")
	}

	headerName := cfg.HeaderName
	if headerName == "" {
		headerName = "X-Forwarded-For"
	}

	return &CleanXFF{
		next:       next,
		name:       name,
		trusted:    nets,
		headerName: headerName,
	}, nil
}

// ServeHTTP implements http.Handler.
func (c *CleanXFF) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	values := req.Header.Values(c.headerName)
	if len(values) == 0 {
		c.next.ServeHTTP(rw, req)
		return
	}

	// The XFF header may appear multiple times and each occurrence may
	// contain a comma-separated list of IPs. Flatten and filter.
	kept := make([]string, 0, 8)
	for _, v := range values {
		for _, p := range strings.Split(v, ",") {
			token := strings.TrimSpace(p)
			if token == "" {
				continue
			}
			ip := net.ParseIP(token)
			if ip == nil {
				// Keep non-IP tokens as-is (paranoid fallback).
				kept = append(kept, token)
				continue
			}
			if !c.isTrusted(ip) {
				kept = append(kept, ip.String())
			}
		}
	}

	req.Header.Del(c.headerName)
	if len(kept) > 0 {
		req.Header.Set(c.headerName, strings.Join(kept, ", "))
	}

	c.next.ServeHTTP(rw, req)
}

// isTrusted reports whether ip is contained in any of the configured CIDRs.
func (c *CleanXFF) isTrusted(ip net.IP) bool {
	for _, n := range c.trusted {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}
