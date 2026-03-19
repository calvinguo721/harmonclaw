// Package main is the HarmonClaw entry point.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"expvar"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"syscall"
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

const version = "v0.2"

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
	var rulesSHA, configSHA string
	{
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			if data, err := os.ReadFile(".cursorrules"); err == nil {
				h := sha256.Sum256(data)
				rulesSHA = hex.EncodeToString(h[:])
			} else {
				rulesSHA = "unavailable"
			}
		}()
		go func() {
			defer wg.Done()
			if data, err := os.ReadFile(configPath); err == nil {
				h := sha256.Sum256(data)
				configSHA = hex.EncodeToString(h[:])
			} else {
				configSHA = "unavailable"
			}
		}()
		wg.Wait()
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
	if sc, err := governor.LoadSovereigntyConfig(cfg.SovereigntyPath); err == nil {
		hclog.Infof("", "sovereignty: loaded from %s (mode=%s)", cfg.SovereigntyPath, sc.Mode)
		sovMode = sc.Mode
		switch governor.ResolveMode(sc.Mode) {
		case string(governor.ModeConnected):
			sovDomains = sc.Connected.Whitelist
		case string(governor.ModeLocal):
			sovDomains = sc.Local.AllowedEndpoints
		default:
			sovDomains = []string{}
		}
	} else {
		// Legacy format fallback
		if data, err := os.ReadFile(cfg.SovereigntyPath); err == nil {
			var sov struct {
				Version string `json:"version"`
				Modes   map[string]struct {
					Desc    string   `json:"description"`
					Domains []string `json:"allowed_domains"`
				} `json:"modes"`
			}
			if json.Unmarshal(data, &sov) == nil {
				hclog.Infof("", "sovereignty: loaded legacy from %s", cfg.SovereigntyPath)
				if m, ok := sov.Modes[sovMode]; ok {
					sovDomains = m.Domains
				}
			}
		} else {
			hclog.Infof("", "sovereignty: %s (using default)", err)
		}
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
	b := butler.NewWithOpts(provider, mem, ledger, cfg.VikingBaseDir(), "configs/persona.json")
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

	// --- Viking store, search, snapshot ---
	kvStore := viking.NewKVStore()
	searchIdx := viking.NewSearchIndexWithPath(filepath.Join(cfg.DataDir, "viking", "index.jsonl"))
	snapDir := filepath.Join(cfg.DataDir, "viking", "snapshots")
	snapMgr := viking.NewSnapshotManager(snapDir, cfg.VikingBaseDir(), 24)
	gc := viking.NewGC(kvStore, snapMgr, filepath.Join(cfg.VikingBaseDir(), "engrams"), ledger)
	gc.Start()

	// --- config watcher (hot-reload) ---
	watcher := configs.NewWatcher("configs")
	go watcher.Start()

	// --- gateway ---
	addr := ":" + cfg.Port
	srv := gateway.NewWithEngramDir(addr, gov, b, a, ledger, policies, version, cfg.VikingBaseDir())
	srv.VikingStore = kvStore
	srv.VikingSearch = searchIdx
	srv.VikingSnap = snapMgr
	rlCfg, _ := governor.LoadRateLimitConfig("configs/ratelimit.json")
	srv.SetRateLimiter(governor.NewTripleRateLimiter(rlCfg))
	srv.SetFirewall(governor.NewFirewall(ledger))
	certFile := os.Getenv("HC_TLS_CERT")
	keyFile := os.Getenv("HC_TLS_KEY")
	if certFile != "" && keyFile != "" {
		hclog.Infof("", "HarmonClaw listening on %s (TLS) [sovereignty=%s]", srv.Addr, gateway.SovereigntyMode)
	} else {
		hclog.Infof("", "HarmonClaw listening on %s [sovereignty=%s]", srv.Addr, gateway.SovereigntyMode)
	}

	// --- graceful shutdown ---
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		hclog.Infof("", "shutdown signal received, draining...")

		if err := srv.Shutdown(30 * time.Second); err != nil {
			hclog.Infof("", "server shutdown: %v", err)
		}
		if a.Crons() != nil {
			a.Crons().Stop()
		}
		if snapMgr != nil {
			if p, err := snapMgr.Snapshot(); err == nil {
				hclog.Infof("", "viking snapshot: %s", p)
			}
		}
		ledger.Record(viking.LedgerEntry{
			OperatorID: "gateway",
			ActionType: "system shutdown",
			Resource:   "main",
			Result:     "success",
			Timestamp:  time.Now().Format(time.RFC3339),
		})
		ledger.Close()
		os.Exit(0)
	}()

	if err := srv.ListenAndServeTLS(certFile, keyFile); err != nil && err != http.ErrServerClosed {
		hclog.Fatal("server died: %v", err)
	}
}
