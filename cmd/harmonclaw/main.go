package main

import (
	"fmt"
	"log"
	"os"

	"harmonclaw/architect"
	"harmonclaw/butler"
	"harmonclaw/gateway"
	"harmonclaw/governor"
	"harmonclaw/llm"
	"harmonclaw/sandbox"
	"harmonclaw/viking"
)

func main() {
	// --- infrastructure ---
	provider, err := llm.NewDeepSeekClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}

	mem, err := viking.NewFileStore()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}

	ledger, err := viking.NewFileLedger()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
	defer ledger.Close()

	guard := sandbox.NewWhitelist()

	// --- three-body agents ---
	b := butler.New(provider, mem, ledger)
	a := architect.New(guard, ledger)
	gov := governor.New(ledger)

	// grant wiring: butler & architect request authorization through governor
	b.SetGrantFunc(gov.RequestGrant)
	a.SetGrantFunc(gov.RequestGrant)

	// heartbeat wiring: governor watches and self-heals
	gov.WatchAgent("butler", b.Heartbeat(), func() { b.Stop(); b.Start() })
	gov.WatchAgent("architect", a.Heartbeat(), func() { a.Stop(); a.Start() })

	// ignition sequence
	b.Start()
	a.Start()
	gov.Start()

	// --- gateway ---
	srv := gateway.New(":8080", b, a, ledger)
	log.Printf("HarmonClaw listening on %s  [sovereignty=%s]", srv.Addr, gateway.SovereigntyMode)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("server died: %v", err)
	}
}
