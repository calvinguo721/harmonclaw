// Package governor (quota) provides per-user and per-skill rate limiting.
package governor

import (
	"runtime"
	"sync"
	"time"
)

const (
	perUserConcurrency = 3
	perSkillQPS        = 10
	goroutineCap       = 500
)

type Quota struct {
	mu sync.Mutex

	userActive map[string]int
	skillTimes map[string][]time.Time
}

func NewQuota() *Quota {
	return &Quota{
		userActive: make(map[string]int),
		skillTimes: make(map[string][]time.Time),
	}
}

func (q *Quota) Allow(userID, skillID string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	if runtime.NumGoroutine() > goroutineCap {
		return false
	}

	if q.userActive[userID] >= perUserConcurrency {
		return false
	}

	now := time.Now()
	cutoff := now.Add(-time.Second)
	times := q.skillTimes[skillID]
	j := 0
	for _, t := range times {
		if t.After(cutoff) {
			times[j] = t
			j++
		}
	}
	times = times[:j]
	if len(times) >= perSkillQPS {
		return false
	}

	q.userActive[userID]++
	q.skillTimes[skillID] = append(times, now)
	return true
}

func (q *Quota) Release(userID string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.userActive[userID] > 0 {
		q.userActive[userID]--
		if q.userActive[userID] == 0 {
			delete(q.userActive, userID)
		}
	}
}
