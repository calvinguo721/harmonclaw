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
)

func InitSecureClient(ledger viking.Ledger, mode string, allowedDomains []string) {
	secureClientOnce.Do(func() {
		clientLedger = ledger
		clientDomains = allowedDomains
		clientMode = mode
		clientInitDone = true
		secureClientVal = &http.Client{
			Transport: &sovereigntyTransport{
				next: &countingTransport{next: http.DefaultTransport},
			},
		}
	})
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

func (t *sovereigntyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if clientLedger != nil && req.URL != nil {
		host := req.URL.Host
		if idx := strings.Index(host, ":"); idx >= 0 {
			host = host[:idx]
		}
		if !domainAllowed(host) {
			clientLedger.Record(viking.LedgerEntry{
				OperatorID: "governor",
				ActionType: "outbound_blocked",
				Resource:   host,
				Result:     "fail",
				ClientIP:   "",
				Timestamp:  time.Now().Format(time.RFC3339),
				ActionID:   "",
			})
			return nil, errors.New("sovereignty: domain " + host + " not in whitelist")
		}
	}
	if t.next == nil {
		t.next = http.DefaultTransport
	}
	return t.next.RoundTrip(req)
}

func domainAllowed(host string) bool {
	if clientMode == "opensea" {
		for _, d := range clientDomains {
			if d == "*" {
				return true
			}
		}
	}
	if clientMode == "shadow" {
		return false
	}
	for _, d := range clientDomains {
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
