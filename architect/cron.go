// Package architect (cron) provides cron expr, time.Ticker, configs/crons.json.
package architect

import (
	"encoding/json"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// CronJob defines a scheduled job.
type CronJob struct {
	ID      string            `json:"id"`
	Expr    string            `json:"expr"` // "0 * * * *" = min hour day month dow
	SkillID string            `json:"skill_id"`
	Args    map[string]string `json:"args"`
}

// CronStore loads from configs/crons.json.
type CronStore struct {
	mu    sync.RWMutex
	path  string
	jobs  []CronJob
	stop  chan struct{}
}

// NewCronStore loads from path.
func NewCronStore(path string) (*CronStore, error) {
	if path == "" {
		path = "configs/crons.json"
	}
	cs := &CronStore{path: path, jobs: nil, stop: make(chan struct{})}
	if err := cs.Load(); err != nil {
		return cs, err
	}
	return cs, nil
}

// Load reads crons.json.
func (c *CronStore) Load() error {
	data, err := os.ReadFile(c.path)
	if err != nil {
		if os.IsNotExist(err) {
			c.mu.Lock()
			c.jobs = []CronJob{}
			c.mu.Unlock()
			return nil
		}
		return err
	}
	var raw struct {
		Crons []CronJob `json:"crons"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	c.mu.Lock()
	c.jobs = raw.Crons
	if c.jobs == nil {
		c.jobs = []CronJob{}
	}
	c.mu.Unlock()
	return nil
}

// List returns all cron jobs.
func (c *CronStore) List() []CronJob {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]CronJob, len(c.jobs))
	copy(out, c.jobs)
	return out
}

// cronExprRe parses "min hour day month dow" (0-5 fields).
var cronExprRe = regexp.MustCompile(`^(\d+|\*)\s+(\d+|\*)\s+(\d+|\*)\s+(\d+|\*)\s+(\d+|\*)$`)

// MatchCron returns true if expr matches now. Supports "min hour * * *" (min,hour) and "* * * * *" (every min).
func MatchCron(expr string, now time.Time) bool {
	parts := strings.Fields(expr)
	if len(parts) < 5 {
		return false
	}
	minMatch := parts[0] == "*" || parts[0] == strconv.Itoa(now.Minute())
	hourMatch := parts[1] == "*" || parts[1] == strconv.Itoa(now.Hour())
	return minMatch && hourMatch
}

// Start runs ticker and invokes exec for each job when due.
func (c *CronStore) Start(exec func(CronJob)) {
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-c.stop:
				return
			case now := <-ticker.C:
				for _, job := range c.List() {
					if MatchCron(job.Expr, now) {
						go exec(job)
					}
				}
			}
		}
	}()
}

// Stop stops the cron loop.
func (c *CronStore) Stop() {
	select {
	case <-c.stop:
	default:
		close(c.stop)
	}
}
