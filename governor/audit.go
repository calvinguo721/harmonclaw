// Package governor (audit) provides audit engine with query and CSV export.
package governor

import (
	"encoding/csv"
	"fmt"
	"io"
	"strings"
	"time"

	"harmonclaw/viking"
)

// AuditEngine provides query and export for ledger entries.
type AuditEngine struct {
	ledger viking.QueryableLedger
}

// NewAuditEngine creates an audit engine.
func NewAuditEngine(ledger viking.QueryableLedger) *AuditEngine {
	return &AuditEngine{ledger: ledger}
}

// QueryFilter for audit queries.
type QueryFilter struct {
	TimeFrom   time.Time
	TimeTo     time.Time
	OperatorID string
	ActionType string
}

// Query returns entries matching the filter.
func (a *AuditEngine) Query(f QueryFilter) ([]viking.LedgerEntry, error) {
	lf := viking.LedgerQueryFilter{
		OperatorID: f.OperatorID,
		ActionType: f.ActionType,
	}
	if !f.TimeFrom.IsZero() {
		lf.TimeFrom = f.TimeFrom.Format(time.RFC3339)
	}
	if !f.TimeTo.IsZero() {
		lf.TimeTo = f.TimeTo.Format(time.RFC3339)
	}
	return a.ledger.Query(lf)
}

// ExportCSV writes entries to w in CSV format.
func (a *AuditEngine) ExportCSV(entries []viking.LedgerEntry, w io.Writer) error {
	cw := csv.NewWriter(w)
	if err := cw.Write([]string{"timestamp", "operator_id", "action_type", "resource", "result", "client_ip", "action_id"}); err != nil {
		return err
	}
	for _, e := range entries {
		row := []string{
			e.Timestamp,
			e.OperatorID,
			e.ActionType,
			e.Resource,
			e.Result,
			e.ClientIP,
			e.ActionID,
		}
		if err := cw.Write(row); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

// ExportCSVString returns CSV as string.
func (a *AuditEngine) ExportCSVString(entries []viking.LedgerEntry) (string, error) {
	var sb strings.Builder
	if err := a.ExportCSV(entries, &sb); err != nil {
		return "", err
	}
	return sb.String(), nil
}

// QueryAndExport runs query and exports to CSV.
func (a *AuditEngine) QueryAndExport(f QueryFilter, w io.Writer) (int, error) {
	entries, err := a.Query(f)
	if err != nil {
		return 0, fmt.Errorf("query: %w", err)
	}
	if err := a.ExportCSV(entries, w); err != nil {
		return 0, fmt.Errorf("export: %w", err)
	}
	return len(entries), nil
}
