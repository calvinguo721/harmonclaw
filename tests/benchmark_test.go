package tests

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"harmonclaw/governor"
	"harmonclaw/viking"
)

func BenchmarkFirewall_Wrap(b *testing.B) {
	dir := b.TempDir()
	ledger, _ := viking.NewFileLedger(dir)
	defer ledger.Close()
	fw := governor.NewFirewall(ledger)
	handler := fw.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	req := httptest.NewRequest("GET", "/v1/health", nil)
	rr := httptest.NewRecorder()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		handler.ServeHTTP(rr, req)
	}
}

func BenchmarkLedger_Record(b *testing.B) {
	dir := b.TempDir()
	ledger, _ := viking.NewFileLedger(dir)
	defer ledger.Close()
	entry := viking.LedgerEntry{
		OperatorID: "bench",
		ActionType: "test",
		Resource:   "/v1/health",
		Result:     "success",
		Timestamp:  "2026-03-07T00:00:00Z",
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ledger.Record(entry)
	}
}
