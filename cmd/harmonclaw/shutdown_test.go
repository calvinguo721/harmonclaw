// Package main tests graceful shutdown components.
package main

import (
	"context"
	"net/http"
	"testing"
	"time"

	"harmonclaw/architect"
	"harmonclaw/butler"
	"harmonclaw/configs"
	"harmonclaw/gateway"
	"harmonclaw/governor"
	"harmonclaw/governor/ironclaw"
	"harmonclaw/llm"
	"harmonclaw/sandbox"
	"harmonclaw/viking"
	hctest "harmonclaw/pkg/testutil"
)

func TestShutdown_GatewayShutdown(t *testing.T) {
	ledger, _ := viking.NewFileLedger(t.TempDir())
	defer ledger.Close()
	provider, _ := llm.NewProvider()
	mem, _ := viking.NewFileStore(t.TempDir())
	guard := sandbox.NewWhitelist()
	policies, _ := ironclaw.LoadPolicies(hctest.ConfigPath("policies.json"))
	governor.InitSecureClient(ledger, "airlock", []string{"*"})

	gov := governor.New(ledger)
	bl := butler.New(provider, mem, ledger)
	a := architect.New(guard, ledger)
	bl.SetGrantFunc(gov.RequestGrant)
	a.SetGrantFunc(gov.RequestGrant)

	srv := gateway.New(":0", gov, bl, a, ledger, policies, "test")
	done := make(chan error, 1)
	go func() {
		done <- srv.ListenAndServe()
	}()
	time.Sleep(50 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := srv.Shutdown(1 * time.Second); err != nil {
		t.Errorf("Shutdown: %v", err)
	}
	select {
	case err := <-done:
		if err != nil && err != http.ErrServerClosed {
			t.Errorf("ListenAndServe: %v", err)
		}
	case <-ctx.Done():
		t.Error("server did not stop in time")
	}
}

func TestShutdown_ConfigWatcherStop(t *testing.T) {
	w := configs.NewWatcher(t.TempDir())
	done := make(chan struct{})
	go func() {
		w.Start()
		close(done)
	}()
	time.Sleep(20 * time.Millisecond)
	w.Stop()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Error("watcher did not stop")
	}
}

func TestShutdown_CronStop(t *testing.T) {
	cs, err := architect.NewCronStore(hctest.ConfigPath("crons.json"))
	if err != nil {
		t.Skip("no crons.json")
	}
	cs.Start(func(_ architect.CronJob) {})
	cs.Stop()
	time.Sleep(50 * time.Millisecond)
}
