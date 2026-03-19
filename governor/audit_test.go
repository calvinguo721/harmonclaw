package governor

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"harmonclaw/viking"
)

func TestAuditEngine_QueryAndExport(t *testing.T) {
	dir := filepath.Join(os.TempDir(), "harmonclaw-audit-test")
	os.MkdirAll(dir, 0755)
	defer os.RemoveAll(dir)
	ledger, err := viking.NewFileLedger(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	ledger.Record(viking.LedgerEntry{
		OperatorID: "user1",
		ActionType: "chat",
		Resource:   "chat",
		Result:     "success",
		Timestamp:  time.Now().Format(time.RFC3339),
		ActionID:   "aid-1",
	})
	time.Sleep(200 * time.Millisecond) // allow drain to write
	var ledgerIf viking.Ledger = ledger
	ql, ok := ledgerIf.(viking.QueryableLedger)
	if !ok {
		t.Skip("ledger does not implement QueryableLedger")
	}
	engine := NewAuditEngine(ql)
	var buf bytes.Buffer
	n, err := engine.QueryAndExport(QueryFilter{OperatorID: "user1"}, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if n < 1 {
		t.Errorf("want at least 1 entry, got %d", n)
	}
	if buf.Len() == 0 {
		t.Error("CSV should not be empty")
	}
}
