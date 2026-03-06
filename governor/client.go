package governor

import (
	"expvar"
	"net/http"
	"sync"
)

var (
	outboundHTTPTotal = expvar.NewInt("outbound_http_total")
	secureClientOnce  sync.Once
	secureClientVal   *http.Client
)

func SecureClient() *http.Client {
	secureClientOnce.Do(func() {
		secureClientVal = &http.Client{
			Transport: &countingTransport{next: http.DefaultTransport},
		}
	})
	return secureClientVal
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
