// Package governor provides CredentialProxy for injecting secrets into outbound API calls
// without exposing credentials to the container. The proxy forwards requests to upstream
// and injects API keys or OAuth tokens based on authMode.
package governor

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
)

// CredentialProxy forwards HTTP requests to upstream and injects credentials.
// Container never touches real secrets; API calls go through this proxy.
type CredentialProxy struct {
	secrets  map[string]string
	upstream *url.URL
	server   *http.Server
	port     int
	authMode string
	mu       sync.RWMutex
}

// NewCredentialProxy creates a proxy that injects credentials into upstream requests.
// authMode: "api-key" injects x-api-key, "oauth" injects Authorization Bearer.
func NewCredentialProxy(upstreamURL string, port int, authMode string) (*CredentialProxy, error) {
	u, err := url.Parse(strings.TrimSuffix(upstreamURL, "/"))
	if err != nil {
		return nil, fmt.Errorf("parse upstream URL: %w", err)
	}
	secrets := loadSecrets()
	return &CredentialProxy{
		secrets:  secrets,
		upstream: u,
		port:     port,
		authMode: authMode,
	}, nil
}

// loadSecrets reads secrets from environment (e.g. API_KEY, OAUTH_TOKEN).
func loadSecrets() map[string]string {
	m := make(map[string]string)
	for _, key := range []string{"API_KEY", "OAUTH_TOKEN", "OPENCLAW_API_TOKEN"} {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			m[key] = v
		}
	}
	return m
}

// Start runs the proxy HTTP server. Blocks until server stops.
func (p *CredentialProxy) Start(host string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", p.handleRequest)
	p.server = &http.Server{
		Addr:    fmt.Sprintf("%s:%d", host, p.port),
		Handler: mux,
	}
	return p.server.ListenAndServe()
}

// handleRequest proxies the request to upstream and injects credentials.
func (p *CredentialProxy) handleRequest(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
		return
	}
	r.Body.Close()

	headers := copyHeaders(r.Header)
	p.mu.RLock()
	secrets := p.secrets
	authMode := p.authMode
	upstream := p.upstream
	p.mu.RUnlock()

	switch authMode {
	case "api-key":
		headers.Del("x-api-key")
		if k, ok := secrets["API_KEY"]; ok {
			headers.Set("x-api-key", k)
		}
	case "oauth":
		headers.Del("Authorization")
		if t, ok := secrets["OAUTH_TOKEN"]; ok {
			headers.Set("Authorization", "Bearer "+t)
		}
	}

	upstreamURL := upstream.ResolveReference(r.URL)

	req, err := http.NewRequest(r.Method, upstreamURL.String(), bytes.NewReader(body))
	if err != nil {
		http.Error(w, "new request: "+err.Error(), http.StatusInternalServerError)
		return
	}
	req.Header = headers

	client := SecureClient()
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, "upstream: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	for k, v := range resp.Header {
		for _, vv := range v {
			w.Header().Add(k, vv)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// copyHeaders copies headers, excluding hop-by-hop headers.
func copyHeaders(h http.Header) http.Header {
	out := make(http.Header)
	skip := map[string]bool{
		"Connection": true, "Keep-Alive": true, "Proxy-Authenticate": true,
		"Proxy-Authorization": true, "Te": true, "Trailers": true,
		"Transfer-Encoding": true, "Upgrade": true,
	}
	for k, v := range h {
		if skip[k] {
			continue
		}
		for _, vv := range v {
			out.Add(k, vv)
		}
	}
	return out
}
