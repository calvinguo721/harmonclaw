// Package governor provides CredentialProxy: container never touches real credentials.
// API calls inject secrets via proxy. IRON RULE #5: all outbound uses SecureClient().
package governor

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// CredentialProxy forwards requests to upstream, injecting API keys from env.
// Secrets are loaded from .env or environment; container mounts /dev/null over .env at runtime.
type CredentialProxy struct {
	secrets  map[string]string
	upstream *url.URL
	server   *http.Server
	port     int
	authMode string // "api-key" or "oauth"
}

// NewCredentialProxy creates a proxy that injects credentials into upstream requests.
func NewCredentialProxy(upstreamURL string, port int, authMode string) (*CredentialProxy, error) {
	u, err := url.Parse(upstreamURL)
	if err != nil {
		return nil, fmt.Errorf("parse upstream: %w", err)
	}
	secrets := loadSecrets()
	return &CredentialProxy{
		secrets:  secrets,
		upstream: u,
		port:     port,
		authMode: authMode,
	}, nil
}

// loadSecrets reads ANTHROPIC_API_KEY, DEEPSEEK_API_KEY, etc. from .env or env.
func loadSecrets() map[string]string {
	m := make(map[string]string)
	keys := []string{
		"ANTHROPIC_API_KEY", "DEEPSEEK_API_KEY", "OPENAI_API_KEY",
		"API_KEY", "OAUTH_TOKEN", "HC_CREDENTIAL_PROXY_KEY",
	}
	for _, k := range keys {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			m[k] = v
		}
	}
	// Also load from .env if present
	paths := []string{".env"}
	if wd, _ := os.Getwd(); wd != "" {
		paths = append(paths, filepath.Join(wd, ".env"))
	}
	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			if idx := strings.Index(line, "="); idx > 0 {
				k := strings.TrimSpace(line[:idx])
				v := strings.TrimSpace(line[idx+1:])
				v = strings.Trim(v, "\"'")
				if k != "" && v != "" {
					m[k] = v
				}
			}
		}
		f.Close()
		break
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

// Shutdown gracefully stops the proxy server.
func (p *CredentialProxy) Shutdown() error {
	if p.server == nil {
		return nil
	}
	return p.server.Close()
}

func (p *CredentialProxy) handleRequest(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}
	r.Body.Close()

	// Copy headers, excluding auth (we inject our own)
	headers := make(http.Header)
	for k, v := range r.Header {
		lower := strings.ToLower(k)
		if lower == "authorization" || lower == "x-api-key" {
			continue
		}
		headers[k] = v
	}

	// Inject credential per IRON RULE #5
	switch p.authMode {
	case "api-key":
		key := p.secrets["API_KEY"]
		if key == "" {
			key = p.secrets["DEEPSEEK_API_KEY"]
		}
		if key == "" {
			key = p.secrets["ANTHROPIC_API_KEY"]
		}
		if key == "" {
			key = p.secrets["OPENAI_API_KEY"]
		}
		if key != "" {
			headers.Set("Authorization", "Bearer "+key)
		}
	case "oauth":
		if token := p.secrets["OAUTH_TOKEN"]; token != "" {
			headers.Set("Authorization", "Bearer "+token)
		}
	default:
		if key := p.secrets["API_KEY"]; key != "" {
			headers.Set("Authorization", "Bearer "+key)
		}
	}

	upstreamURL := p.upstream.ResolveReference(&url.URL{Path: r.URL.Path, RawQuery: r.URL.RawQuery})
	req, err := http.NewRequest(r.Method, upstreamURL.String(), bytes.NewReader(body))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	req.Header = headers
	req.ContentLength = int64(len(body))

	resp, err := SecureClient().Do(req)
	if err != nil {
		http.Error(w, "upstream error: "+err.Error(), http.StatusBadGateway)
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
