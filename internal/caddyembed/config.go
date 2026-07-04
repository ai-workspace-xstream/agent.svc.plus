package caddyembed

import (
	"fmt"
	"strings"
)

// StandaloneOptions describes the single-node "Caddy manages my own
// certificate" topology used on VPS/standalone deployments (see
// scripts/setup-proxy.sh's generated /etc/caddy/Caddyfile, which this
// replaces). It intentionally does not apply to Cloud Run or Cloudflare
// Containers deployments: on those platforms TLS is already terminated by
// the platform's own edge, so Caddy is not started at all (see
// docs/architecture/proxy-server/monolith-embed-plan.md, "Runtime modes").
type StandaloneOptions struct {
	// Domain is the public hostname Caddy will request a certificate for.
	Domain string

	// DNSProvider selects the ACME DNS-01 challenge provider: "cloudflare"
	// or "alidns". DNS-01 (rather than HTTP-01) is used so issuance works
	// even when the domain is CDN-proxied and port 80 isn't reachable.
	DNSProvider  string
	DNSAPIToken  string // cloudflare: api_token
	AliKeyID     string // alidns: access_key_id
	AliKeySecret string // alidns: access_key_secret

	// XHTTPSocket is the unix socket the embedded xhttp Xray instance
	// listens on (see config/xray.xhttp.template.json). Caddy terminates
	// TLS on :443 and reverse-proxies the XHTTP path to it over h2c.
	XHTTPSocket string
}

// BuildStandaloneConfig renders the Caddy JSON config (the load-bearing
// input to Apply) for the standalone/VPS runtime mode: automatic HTTPS for
// Domain via DNS-01, and a reverse proxy from the public :443 xhttp path to
// the embedded Xray instance's unix socket.
func BuildStandaloneConfig(opts StandaloneOptions) (map[string]any, error) {
	domain := strings.TrimSpace(opts.Domain)
	if domain == "" {
		return nil, fmt.Errorf("caddyembed: domain is required")
	}
	socket := strings.TrimSpace(opts.XHTTPSocket)
	if socket == "" {
		socket = "/dev/shm/xray.sock"
	}

	provider, err := buildDNSProvider(opts)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"apps": map[string]any{
			"tls": map[string]any{
				"automation": map[string]any{
					"policies": []map[string]any{
						{
							"subjects": []string{domain},
							"issuers": []map[string]any{
								{
									"module": "acme",
									"challenges": map[string]any{
										"dns": map[string]any{
											"provider": provider,
										},
									},
								},
							},
						},
					},
				},
			},
			"http": map[string]any{
				"servers": map[string]any{
					"srv0": map[string]any{
						"listen": []string{":443"},
						"routes": []map[string]any{
							{
								"match": []map[string]any{
									{"path": []string{"/split/*"}},
								},
								"handle": []map[string]any{
									{
										"handler": "reverse_proxy",
										"transport": map[string]any{
											"protocol": "http",
											"versions": []string{"h2c", "2"},
										},
										"upstreams": []map[string]any{
											{"dial": "unix/" + socket},
										},
									},
								},
							},
							{
								"handle": []map[string]any{
									{
										"handler": "static_response",
										"body":    "Agent Service Plus Node",
									},
								},
							},
						},
					},
				},
			},
		},
	}, nil
}

func buildDNSProvider(opts StandaloneOptions) (map[string]any, error) {
	switch strings.ToLower(strings.TrimSpace(opts.DNSProvider)) {
	case "cloudflare":
		if opts.DNSAPIToken == "" {
			return nil, fmt.Errorf("caddyembed: cloudflare dns provider requires an api token")
		}
		return map[string]any{
			"name":      "cloudflare",
			"api_token": opts.DNSAPIToken,
		}, nil
	case "alidns":
		if opts.AliKeyID == "" || opts.AliKeySecret == "" {
			return nil, fmt.Errorf("caddyembed: alidns provider requires access_key_id and access_key_secret")
		}
		return map[string]any{
			"name":              "alidns",
			"access_key_id":     opts.AliKeyID,
			"access_key_secret": opts.AliKeySecret,
		}, nil
	default:
		return nil, fmt.Errorf("caddyembed: unsupported dns provider %q (want cloudflare or alidns)", opts.DNSProvider)
	}
}
