package main

import (
	"crypto/sha256"
	"encoding/hex"
	"expvar"
	"fmt"
	"log"
	"os"
	"runtime"
	"sort"
	"strings"
	"time"

	"harmonclaw/architect"
	"harmonclaw/bus"
	"harmonclaw/butler"
	"harmonclaw/gateway"
	"harmonclaw/governor"
	"harmonclaw/governor/ironclaw"
	"harmonclaw/llm"
	"harmonclaw/sandbox"
	"harmonclaw/skills"
	"harmonclaw/viking"

	_ "harmonclaw/skills/doc_perceiver"
	_ "harmonclaw/skills/mimicclaw_adapter"
	_ "harmonclaw/skills/openclaw_adapter"
	_ "harmonclaw/skills/web_search"
	_ "harmonclaw/skills/tts"
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
	skillList := make([]string, 0, len(skills.Registry))
	for id := range skills.Registry {
		skillList = append(skillList, id)
	}
	sort.Strings(skillList)

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

	var policies []ironclaw.Policy
	if path := os.Getenv("HC_IRONCLAW_POLICIES"); path != "" {
		var err error
		policies, err = ironclaw.LoadPolicies(path)
		if err != nil {
			log.Printf("ironclaw: load policies failed: %v", err)
		}
	}

	// --- three cores: Governor → Butler → Architect ---
	gov := governor.New(ledger)
	b := butler.New(provider, mem, ledger)
	a := architect.New(guard, ledger)

	b.SetGrantFunc(gov.RequestGrant)
	a.SetGrantFunc(gov.RequestGrant)

	a.Pool().Start()

	// --- pulse heartbeats ---
	go gov.Pulse()
	go b.Pulse()
	go a.Pulse()

	// --- bus monitor: 15s no pulse → degraded ---
	lastPulse := map[bus.CoreID]time.Time{
		bus.Governor:  time.Now(),
		bus.Butler:    time.Now(),
		bus.Architect: time.Now(),
	}
	go func() {
		ch := bus.Subscribe()
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case m := <-ch:
				if m.Type == "pulse" {
					lastPulse[m.From] = time.Now()
					switch m.From {
					case bus.Governor:
						gov.SetOK()
					case bus.Butler:
						b.SetOK()
					case bus.Architect:
						a.SetOK()
					}
				}
			case <-ticker.C:
				now := time.Now()
				for core, t := range lastPulse {
					if now.Sub(t) > 15*time.Second {
						switch core {
						case bus.Governor:
							gov.SetDegraded()
						case bus.Butler:
							b.SetDegraded()
						case bus.Architect:
							a.SetDegraded()
						}
					}
				}
			}
		}
	}()

	// --- boot log ---
	log.Printf("[BOOT] version=%s rules_sha256=%s skills=[%s] cores=[governor:%s, butler:%s, architect:%s]",
		version, rulesSHA, strings.Join(skillList, ", "),
		gov.Status(), b.Status(), a.Status())

	// --- gateway ---
	srv := gateway.New(":8080", gov, b, a, ledger, policies, version)
	log.Printf("HarmonClaw listening on %s  [sovereignty=%s]", srv.Addr, gateway.SovereigntyMode)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("server died: %v", err)
	}
}
