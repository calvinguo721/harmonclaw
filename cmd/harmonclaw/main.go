// Package main is the HarmonClaw entry point.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"expvar"
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"
	"time"

	"harmonclaw/architect"
	"harmonclaw/bus"
	"harmonclaw/butler"
	"harmonclaw/configs"
	"harmonclaw/gateway"
	"harmonclaw/governor"
	"harmonclaw/governor/ironclaw"
	"harmonclaw/llm"
	hclog "harmonclaw/pkg/log"
	"harmonclaw/sandbox"
	"harmonclaw/skills"
	"harmonclaw/viking"

	_ "harmonclaw/skills/doc_perceiver"
	_ "harmonclaw/skills/mimicclaw_adapter"
	_ "harmonclaw/skills/nanoclaw_adapter"
	_ "harmonclaw/skills/openclaw_adapter"
	_ "harmonclaw/skills/picoclaw_adapter"
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
	configPath := os.Getenv("HC_CONFIG")
	if configPath == "" {
		configPath = "configs/config.json"
	}
	cfg, err := configs.Load(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: config load: %v\n", err)
		os.Exit(1)
	}
	hclog.SetLevel(cfg.LogLevel)

	if err := cfg.EnsureDirs(); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: ensure dirs: %v\n", err)
		os.Exit(1)
	}

	// --- boot banner (IRON RULE #8) ---
	rulesSHA := ""
	if data, err := os.ReadFile(".cursorrules"); err == nil {
		h := sha256.Sum256(data)
		rulesSHA = hex.EncodeToString(h[:])
	} else {
		rulesSHA = "unavailable"
	}
	configSHA := ""
	if data, err := os.ReadFile(configPath); err == nil {
		h := sha256.Sum256(data)
		configSHA = hex.EncodeToString(h[:])
	} else {
		configSHA = "unavailable"
	}
	skillList := make([]string, 0, len(skills.Registry))
	for id := range skills.Registry {
		skillList = append(skillList, id)
	}
	sort.Strings(skillList)

	hclog.Infof("", "config_path=%s config_sha256=%s version=%s data_dir=%s", configPath, configSHA, cfg.Version, cfg.DataDir)

	// --- infrastructure ---
	ledger, err := viking.NewFileLedger(cfg.LedgerDir())
	if err != nil {
		hclog.Fatal("ledger init: %v", err)
	}
	defer ledger.Close()

	mem, err := viking.NewFileStore(cfg.VikingBaseDir())
	if err != nil {
		hclog.Fatal("store init: %v", err)
	}

	guard := sandbox.NewWhitelist()

	var policies []ironclaw.Policy
	policies, err = ironclaw.LoadPolicies(cfg.PoliciesPath)
	if err != nil {
		hclog.Infof("", "policies: %s (fallback: empty)", err)
		policies = nil
	} else {
		hclog.Infof("", "policies: loaded %d from %s", len(policies), cfg.PoliciesPath)
	}

	sovMode := cfg.SovereigntyMode
	sovDomains := []string{}
	if data, err := os.ReadFile(cfg.SovereigntyPath); err == nil {
		var sov struct {
			Version string `json:"version"`
			Modes   map[string]struct {
				Desc    string   `json:"description"`
				Domains []string `json:"allowed_domains"`
			} `json:"modes"`
		}
		if json.Unmarshal(data, &sov) == nil {
			hclog.Infof("", "sovereignty: loaded from %s (version=%s)", cfg.SovereigntyPath, sov.Version)
			if m, ok := sov.Modes[sovMode]; ok {
				sovDomains = m.Domains
			}
		}
	} else {
		hclog.Infof("", "sovereignty: %s (using default)", err)
	}
	governor.InitSecureClient(ledger, sovMode, sovDomains)
	gateway.SovereigntyMode = sovMode

	provider, err := llm.NewProvider()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
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
	configsStr := "policies=" + fmt.Sprintf("%d", len(policies)) + " sovereignty=" + sovMode
	hclog.Infof("", "version=%s rules_sha256=%s skills=[%s] cores=[governor:%s, butler:%s, architect:%s] configs=[%s]",
		version, rulesSHA, strings.Join(skillList, ", "),
		gov.Status(), b.Status(), a.Status(), configsStr)

	// --- gateway ---
	addr := ":" + cfg.Port
	srv := gateway.NewWithEngramDir(addr, gov, b, a, ledger, policies, version, cfg.VikingBaseDir())
	hclog.Infof("", "HarmonClaw listening on %s [sovereignty=%s]", srv.Addr, gateway.SovereigntyMode)
	if err := srv.ListenAndServe(); err != nil {
		hclog.Fatal("server died: %v", err)
	}
}
