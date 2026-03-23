// Package governor (client) provides SecureClient with sovereignty whitelist.
package governor

import (
	"errors"
	"expvar"
	"net/http"
	"strings"
	"sync"
	"time"

	"harmonclaw/viking"
)

var (
	outboundHTTPTotal = expvar.NewInt("outbound_http_total")
	secureClientOnce  sync.Once
	secureClientVal   *http.Client

	clientLedger   viking.Ledger
	clientDomains  []string
	clientMode     string
	clientInitDone bool
	clientMu       sync.RWMutex
)

func InitSecureClient(ledger viking.Ledger, mode string, allowedDomains []string) {
	secureClientOnce.Do(func() {
		clientLedger = ledger
		clientDomains = allowedDomains
		clientMode = mode
		clientInitDone = true
		SetSovereigntyConfig(mode, allowedDomains)
		secureClientVal = &http.Client{
			Transport: &sovereigntyTransport{
				next: &countingTransport{next: http.DefaultTransport},
			},
		}
	})
}

// SetSovereigntyMode updates mode and whitelist at runtime. Writes change to Ledger.
func SetSovereigntyMode(mode string, allowedDomains []string) {
	clientMu.Lock()
	oldMode := clientMode
	clientMode = mode
	clientDomains = allowedDomains
	clientMu.Unlock()
	SetSovereigntyConfig(mode, allowedDomains)
	if clientLedger != nil && oldMode != mode {
		clientLedger.Record(viking.LedgerEntry{
			OperatorID: "governor",
			ActionType: "sovereignty_switch",
			Resource:   mode,
			Result:     "success",
			Timestamp:  time.Now().Format(time.RFC3339),
			ActionID:   "",
		})
	}
}

// GetSovereigntyMode returns current mode and whitelist.
func GetSovereigntyMode() (mode string, domains []string) {
	clientMu.RLock()
	defer clientMu.RUnlock()
	domains = make([]string, len(clientDomains))
	copy(domains, clientDomains)
	return clientMode, domains
}

func SecureClient() *http.Client {
	if !clientInitDone {
		secureClientVal = &http.Client{Transport: http.DefaultTransport}
	}
	return secureClientVal
}

type sovereigntyTransport struct {
	next http.RoundTripper
}

func stripHostPort(host string) string {
	if idx := strings.Index(host, ":"); idx >= 0 {
		return host[:idx]
	}
	return host
}

// AllowOutboundHost applies the same host allowlist as SecureClient (when a ledger is configured).
// Use this before non-HTTP outbound calls (e.g. WebSocket) that cannot use SecureClient.RoundTrip.
func AllowOutboundHost(host string) error {
	if clientLedger == nil {
		return nil
	}
	host = stripHostPort(host)
	if host == "" {
		return nil
	}
	if sc := GetSovereigntyConfig(); sc != nil {
		resolved := ResolveMode(sc.Mode)
		if resolved == string(ModePersonal) || resolved == string(ModeLocal) || resolved == string(ModeConnected) {
			if !sc.IsAllowed(host) {
				clientLedger.Record(viking.LedgerEntry{
					OperatorID: "governor",
					ActionType: sc.Mode + ":outbound_blocked",
					Resource:   host,
					Result:     "fail",
					ClientIP:   "",
					Timestamp:  time.Now().Format(time.RFC3339),
					ActionID:   "",
				})
				return errors.New("sovereignty: " + host + " not allowed in " + sc.Mode)
			}
			return nil
		}
	}
	clientMu.RLock()
	mode, domains := clientMode, clientDomains
	clientMu.RUnlock()
	if !domainAllowedLocked(host, mode, domains) {
		clientLedger.Record(viking.LedgerEntry{
			OperatorID: "governor",
			ActionType: mode + ":outbound_blocked",
			Resource:   host,
			Result:     "fail",
			ClientIP:   "",
			Timestamp:  time.Now().Format(time.RFC3339),
			ActionID:   "",
		})
		return errors.New("sovereignty: domain " + host + " not in whitelist")
	}
	return nil
}

func (t *sovereigntyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if clientLedger != nil && req.URL != nil {
		if err := AllowOutboundHost(req.URL.Host); err != nil {
			return nil, err
		}
	}
	if t.next == nil {
		t.next = http.DefaultTransport
	}
	return t.next.RoundTrip(req)
}

func domainAllowedLocked(host, mode string, domains []string) bool {
	if mode == "opensea" {
		for _, d := range domains {
			if d == "*" {
				return true
			}
		}
	}
	if mode == "shadow" {
		return false
	}
	for _, d := range domains {
		if d == "*" {
			return true
		}
		if d == host {
			return true
		}
		if strings.HasPrefix(d, "*.") {
			suffix := d[1:]
			if host == suffix || strings.HasSuffix(host, suffix) {
				return true
			}
		}
	}
	return false
}

type countingTransport struct {
	next http.RoundTripper
}

func (t *countingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	outboundHTTPTotal.Add(1)
	if t.next == nil {
		t.next = http.DefaultTransport
	}
	return t.next.RoundTrip(req)
}

var _ http.RoundTripper = (*sovereigntyTransport)(nil)
