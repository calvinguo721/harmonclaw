package main

import (
	"fmt"
	"log"
	"os"

	"harmonclaw/gateway"
	"harmonclaw/llm"
	"harmonclaw/sandbox"
	"harmonclaw/viking"
)

func main() {
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

	srv := gateway.New(":8080", provider, mem, guard, ledger)
	log.Printf("HarmonClaw listening on %s  [sovereignty=%s]", srv.Addr, gateway.SovereigntyMode)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("server died: %v", err)
	}
}
