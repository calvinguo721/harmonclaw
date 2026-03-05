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

	guard := sandbox.NewWhitelist()

	srv := gateway.New(":8080", provider, mem, guard)
	log.Printf("HarmonClaw listening on %s", srv.Addr)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("server died: %v", err)
	}
}
