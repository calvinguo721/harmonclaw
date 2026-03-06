package main

import (
	"crypto/sha256"
	"encoding/hex"
	"expvar"
	"fmt"
	"log"
	"os"
	"runtime"

	"harmonclaw/architect"
	"harmonclaw/butler"
	"harmonclaw/gateway"
	"harmonclaw/governor"
	"harmonclaw/llm"
	"harmonclaw/sandbox"
	"harmonclaw/viking"

	_ "harmonclaw/skills/doc_perceiver"
	_ "harmonclaw/skills/openclaw_adapter"
	_ "harmonclaw/skills/web_search"
)

func init() {
	expvar.Publish("goroutine_count", expvar.Func(func() any { return runtime.NumGoroutine() }))
	expvar.Publish("heap_alloc_bytes", expvar.Func(func() any {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		return m.HeapAlloc
	}))
}

const version = "v0.1.7"

func main() {
	// --- boot banner (IRON RULE #8) ---
	rulesSHA := ""
	if data, err := os.ReadFile(".cursorrules"); err == nil {
		h := sha256.Sum256(data)
		rulesSHA = hex.EncodeToString(h[:])
	} else {
		rulesSHA = "unavailable"
	}
	log.Printf("[BOOT] version=%s rules_sha256=%s", version, rulesSHA)

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

	// grant wiring
	b.SetGrantFunc(gov.RequestGrant)
	a.SetGrantFunc(gov.RequestGrant)

	// heartbeat wiring
	gov.WatchAgent("butler", b.Heartbeat(), func() { b.Stop(); b.Start() })
	gov.WatchAgent("architect", a.Heartbeat(), func() { a.Stop(); a.Start() })

	// ignition
	b.Start()
	a.Start()
	gov.Start()

	// --- gateway ---
	srv := gateway.New(":8080", gov, b, a, ledger)
	log.Printf("HarmonClaw listening on %s  [sovereignty=%s]", srv.Addr, gateway.SovereigntyMode)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("server died: %v", err)
	}
}
