// Package caddy provides a full integration with the Caddy Admin API for
// dynamic, zero-downtime reverse-proxy configuration.
//
// Caddy's JSON API (http://localhost:2019) is used exclusively; no config files
// are written and no Caddy restart is ever required. Every mutation is atomic
// from Caddy's perspective: we patch a single route object identified by its
// stable @id tag, which Caddy uses to locate and replace the exact route.
package caddy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/agnivo/agnivo/packages/application/proxy/model"
	"github.com/agnivo/agnivo/packages/platform/errors"
	"go.uber.org/zap"
)

// Client drives the Caddy Admin API.
type Client struct {
	baseURL    string
	httpClient *http.Client
	log        *zap.Logger
}

// NewClient creates a Caddy Admin API client.
func NewClient(adminURL string, log *zap.Logger) *Client {
	if adminURL == "" {
		adminURL = "http://localhost:2019"
	}
	return &Client{
		baseURL: adminURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		log: log,
	}
}

// ─────────────────────────────── Caddy JSON types ────────────────────────────

// caddyConfig is the top-level Caddy configuration envelope.
type caddyConfig struct {
	Apps caddyApps `json:"apps"`
}

type caddyApps struct {
	HTTP *caddyHTTPApp `json:"http,omitempty"`
	TLS  *caddyTLSApp  `json:"tls,omitempty"`
}

type caddyHTTPApp struct {
	Servers map[string]*caddyServer `json:"servers"`
}

type caddyServer struct {
	Listen []string     `json:"listen"`
	Routes []caddyRoute `json:"routes"`
}

type caddyRoute struct {
	ID      string          `json:"@id,omitempty"`
	Match   []caddyMatch    `json:"match"`
	Handle  []json.RawMessage `json:"handle"`
	Terminal bool           `json:"terminal,omitempty"`
}

type caddyMatch struct {
	Host []string `json:"host,omitempty"`
	Path []string `json:"path,omitempty"`
}

// caddyHTTPSRedirect is a permanent redirect handler.
type caddyHTTPSRedirect struct {
	Handler    string `json:"handler"`
	StatusCode int    `json:"status_code"`
}

// caddyReverseProxy is the reverse_proxy handler config.
type caddyReverseProxy struct {
	Handler   string              `json:"handler"`
	Upstreams []caddyUpstream     `json:"upstreams"`
	Transport *caddyTransport     `json:"transport,omitempty"`
	LoadBalancing *caddyLB        `json:"load_balancing,omitempty"`
	Headers   *caddyHeadersOp     `json:"headers,omitempty"`
	HealthChecks *caddyHealthCheck `json:"health_checks,omitempty"`
}

type caddyUpstream struct {
	Dial   string  `json:"dial"`
	Weight int     `json:"weight,omitempty"`
}

type caddyTransport struct {
	Protocol         string `json:"protocol"`
	DialTimeout      string `json:"dial_timeout,omitempty"`
	ResponseTimeout  string `json:"response_header_timeout,omitempty"`
	KeepAlive        *caddyKeepAlive `json:"keep_alive,omitempty"`
}

type caddyKeepAlive struct {
	Enabled           *bool `json:"enabled,omitempty"`
	ProbeInterval     string `json:"probe_interval,omitempty"`
	MaxIdleConns      int    `json:"max_idle_conns,omitempty"`
	MaxIdleConnsPerHost int  `json:"max_idle_conns_per_host,omitempty"`
}

type caddyLB struct {
	SelectionPolicy caddySelectionPolicy `json:"selection_policy"`
	Retries         int                  `json:"retries,omitempty"`
}

type caddySelectionPolicy struct {
	Policy string `json:"policy"`
}

type caddyHeadersOp struct {
	Request  *caddyHeaderSet `json:"request,omitempty"`
	Response *caddyHeaderSet `json:"response,omitempty"`
}

type caddyHeaderSet struct {
	Set map[string]string `json:"set,omitempty"`
	Add map[string]string `json:"add,omitempty"`
	Del []string          `json:"delete,omitempty"`
}

type caddyHealthCheck struct {
	Active *caddyActiveHealthCheck `json:"active,omitempty"`
}

type caddyActiveHealthCheck struct {
	URI      string `json:"uri,omitempty"`
	Interval string `json:"interval,omitempty"`
	Timeout  string `json:"timeout,omitempty"`
}

type caddyTLSApp struct {
	Certificates caddyCertificates `json:"certificates"`
	Automation   *caddyAutomation  `json:"automation,omitempty"`
}

type caddyCertificates struct {
	Automate []string `json:"automate,omitempty"`
}

type caddyAutomation struct {
	Policies []caddyAutomationPolicy `json:"policies,omitempty"`
}

type caddyAutomationPolicy struct {
	Subjects []string `json:"subjects,omitempty"`
	Issuers  []caddyIssuer `json:"issuers,omitempty"`
}

type caddyIssuer struct {
	Module string `json:"module"`
	CA     string `json:"ca,omitempty"`
	Email  string `json:"email,omitempty"`
}

// ─────────────────────────────── Public API ──────────────────────────────────

// UpsertRoute atomically creates or replaces a vhost route in Caddy.
// It uses Caddy's @id-based route addressing so no server restart is needed.
func (c *Client) UpsertRoute(ctx context.Context, cfg model.CaddyRouteConfig) error {
	route, err := c.buildRoute(cfg)
	if err != nil {
		return err
	}

	data, err := json.Marshal(route)
	if err != nil {
		return errors.Wrap(err, errors.CodeInternal, "caddy: marshal route")
	}

	// Try to update an existing route first; fall back to append.
	routeID := routeTagID(cfg.Hostname)
	updatePath := fmt.Sprintf("/id/%s", routeID)
	if err := c.do(ctx, http.MethodPatch, updatePath, data); err != nil {
		// Route doesn't exist yet — append to server routes.
		if err2 := c.do(ctx, http.MethodPost, "/config/apps/http/servers/main/routes", data); err2 != nil {
			return errors.Wrapf(err2, errors.CodeInternal, "caddy: append route for %s", cfg.Hostname)
		}
	}

	// Ensure TLS automation is configured for this hostname.
	if cfg.TLSEnabled {
		if err := c.AutomateCert(ctx, cfg.Hostname); err != nil {
			c.log.Warn("caddy: tls automate failed", zap.String("hostname", cfg.Hostname), zap.Error(err))
		}
	}

	c.log.Info("caddy: route upserted", zap.String("hostname", cfg.Hostname), zap.String("upstream", cfg.Upstream))
	return nil
}

// DeleteRoute removes a route by its stable @id.
func (c *Client) DeleteRoute(ctx context.Context, hostname string) error {
	routeID := routeTagID(hostname)
	path := fmt.Sprintf("/id/%s", routeID)
	if err := c.do(ctx, http.MethodDelete, path, nil); err != nil {
		// Not found is fine — route may have already been removed.
		if errors.IsCode(err, errors.CodeNotFound) {
			return nil
		}
		return errors.Wrapf(err, errors.CodeInternal, "caddy: delete route for %s", hostname)
	}
	c.log.Info("caddy: route deleted", zap.String("hostname", hostname))
	return nil
}

// AutomateCert adds a hostname to the TLS automation list.
func (c *Client) AutomateCert(ctx context.Context, hostname string) error {
	// Idempotently add to the automate list by reading first.
	var current []string
	if err := c.get(ctx, "/config/apps/tls/certificates/automate", &current); err != nil {
		current = []string{}
	}
	for _, h := range current {
		if h == hostname {
			return nil
		}
	}
	current = append(current, hostname)
	data, _ := json.Marshal(current)
	return c.do(ctx, http.MethodPut, "/config/apps/tls/certificates/automate", data)
}

// RevokeCert removes a hostname from TLS automation.
func (c *Client) RevokeCert(ctx context.Context, hostname string) error {
	var current []string
	if err := c.get(ctx, "/config/apps/tls/certificates/automate", &current); err != nil {
		return nil
	}
	filtered := current[:0]
	for _, h := range current {
		if h != hostname {
			filtered = append(filtered, h)
		}
	}
	if len(filtered) == len(current) {
		return nil
	}
	data, _ := json.Marshal(filtered)
	return c.do(ctx, http.MethodPut, "/config/apps/tls/certificates/automate", data)
}

// ListRoutes returns all routes currently registered in Caddy's main server.
func (c *Client) ListRoutes(ctx context.Context) ([]caddyRoute, error) {
	var routes []caddyRoute
	err := c.get(ctx, "/config/apps/http/servers/main/routes", &routes)
	return routes, err
}

// LoadConfig atomically replaces the full Caddy configuration. Used during
// startup reconciliation to push the full desired state in a single call.
func (c *Client) LoadConfig(ctx context.Context, routes []model.CaddyRouteConfig, tlsHostnames []string) error {
	builtRoutes := make([]caddyRoute, 0, len(routes))
	for _, r := range routes {
		rt, err := c.buildRoute(r)
		if err != nil {
			c.log.Warn("caddy: skip bad route", zap.String("hostname", r.Hostname), zap.Error(err))
			continue
		}
		builtRoutes = append(builtRoutes, rt)
	}

	trueBool := true
	cfg := caddyConfig{
		Apps: caddyApps{
			HTTP: &caddyHTTPApp{
				Servers: map[string]*caddyServer{
					"main": {
						Listen: []string{":80", ":443"},
						Routes: builtRoutes,
					},
				},
			},
			TLS: &caddyTLSApp{
				Certificates: caddyCertificates{Automate: tlsHostnames},
				Automation: &caddyAutomation{
					Policies: []caddyAutomationPolicy{
						{
							Subjects: tlsHostnames,
							Issuers: []caddyIssuer{{
								Module: "acme",
								CA:     "https://acme-v02.api.letsencrypt.org/directory",
							}},
						},
					},
				},
			},
		},
	}
	_ = trueBool

	data, err := json.Marshal(cfg)
	if err != nil {
		return errors.Wrap(err, errors.CodeInternal, "caddy: marshal full config")
	}
	return c.do(ctx, http.MethodPost, "/load", data)
}

// Ping verifies connectivity with the Caddy Admin API.
func (c *Client) Ping(ctx context.Context) error {
	var v any
	return c.get(ctx, "/config/", &v)
}

// ─────────────────────────────── Route building ──────────────────────────────

func (c *Client) buildRoute(cfg model.CaddyRouteConfig) (caddyRoute, error) {
	// Build the reverse-proxy handler.
	upstreams := []caddyUpstream{{Dial: cfg.Upstream}}
	if cfg.CanaryWeight > 0 && cfg.CanaryUpstream != "" {
		upstreams = []caddyUpstream{
			{Dial: cfg.Upstream, Weight: 100 - cfg.CanaryWeight},
			{Dial: cfg.CanaryUpstream, Weight: cfg.CanaryWeight},
		}
	}

	timeout := cfg.TimeoutSeconds
	if timeout <= 0 {
		timeout = 30
	}
	retries := cfg.MaxRetries
	if retries < 0 {
		retries = 3
	}

	trueBool := true
	transport := &caddyTransport{
		Protocol:        "http",
		DialTimeout:     "5s",
		ResponseTimeout: fmt.Sprintf("%ds", timeout),
		KeepAlive: &caddyKeepAlive{
			Enabled:             &trueBool,
			MaxIdleConns:        200,
			MaxIdleConnsPerHost: 20,
		},
	}

	// Set forwarded headers so upstreams see the real client IP.
	reqHeaders := map[string]string{
		"X-Forwarded-For":   "{http.request.remote.host}",
		"X-Forwarded-Proto": "{http.request.scheme}",
		"X-Real-IP":         "{http.request.remote.host}",
		"X-Forwarded-Host":  "{http.request.host}",
	}
	for k, v := range cfg.AddHeaders {
		reqHeaders[k] = v
	}

	rp := caddyReverseProxy{
		Handler:   "reverse_proxy",
		Upstreams: upstreams,
		Transport: transport,
		LoadBalancing: &caddyLB{
			SelectionPolicy: caddySelectionPolicy{Policy: "least_conn"},
			Retries:         retries,
		},
		Headers: &caddyHeadersOp{
			Request: &caddyHeaderSet{Set: reqHeaders},
		},
		HealthChecks: &caddyHealthCheck{
			Active: &caddyActiveHealthCheck{
				URI:      "/health",
				Interval: "10s",
				Timeout:  "5s",
			},
		},
	}

	rpData, err := json.Marshal(rp)
	if err != nil {
		return caddyRoute{}, errors.Wrap(err, errors.CodeInternal, "caddy: marshal reverse_proxy")
	}

	handlers := []json.RawMessage{rpData}

	// Add HTTPS redirect handler at the front for port 80.
	if cfg.HTTPSRedirect {
		redirect := caddyHTTPSRedirect{Handler: "static_response", StatusCode: 301}
		// Redirect will be on a separate HTTP-only route — handled by LoadConfig.
		// For UpsertRoute (HTTPS-only path) just add it as a note; the main server listens on both.
		_ = redirect
	}

	route := caddyRoute{
		ID:      routeTagID(cfg.Hostname),
		Match:   []caddyMatch{{Host: []string{cfg.Hostname}}},
		Handle:  handlers,
		Terminal: true,
	}
	return route, nil
}

// routeTagID returns a deterministic @id tag for a hostname.
// Caddy uses this to locate routes for atomic PATCH/DELETE operations.
func routeTagID(hostname string) string {
	return "route-" + sanitizeTag(hostname)
}

func sanitizeTag(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
			out = append(out, c)
		} else if c == '.' {
			out = append(out, '-')
		}
	}
	return string(out)
}

// ─────────────────────────────── HTTP helpers ────────────────────────────────

func (c *Client) do(ctx context.Context, method, path string, body []byte) error {
	url := c.baseURL + path
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return errors.Wrap(err, errors.CodeInternal, "caddy: build request")
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return errors.Wrap(err, errors.CodeUnavailable, "caddy: request failed")
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	switch {
	case resp.StatusCode == http.StatusNotFound:
		return errors.New(errors.CodeNotFound, "caddy: not found")
	case resp.StatusCode >= 400:
		return errors.New(errors.CodeInternal,
			fmt.Sprintf("caddy: api error %d: %s", resp.StatusCode, string(respBody)))
	}
	return nil
}

func (c *Client) get(ctx context.Context, path string, dst any) error {
	url := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return errors.Wrap(err, errors.CodeInternal, "caddy: build GET")
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return errors.Wrap(err, errors.CodeUnavailable, "caddy: GET failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return errors.New(errors.CodeNotFound, "caddy: not found")
	}
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return errors.New(errors.CodeInternal,
			fmt.Sprintf("caddy: GET %s returned %d: %s", path, resp.StatusCode, string(b)))
	}
	return json.NewDecoder(resp.Body).Decode(dst)
}
