// Package store keeps the response cache and the persisted daily API budget.
package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ErrBudget is returned when the daily request budget is used up.
var ErrBudget = errors.New("daily request budget used up")

type entry struct {
	body    []byte
	fetched time.Time
}

// Cache is a small TTL cache with per-key locking, so concurrent viewers
// trigger one upstream request. Stale entries are kept as a fallback.
type Cache struct {
	mu    sync.Mutex
	items map[string]entry
	locks map[string]*sync.Mutex
}

func NewCache() *Cache {
	return &Cache{items: map[string]entry{}, locks: map[string]*sync.Mutex{}}
}

// Result says where a body came from.
type Result struct {
	Body    []byte
	Fetched time.Time
	Stale   bool  // upstream failed or budget exhausted; this is the last good copy
	Err     error // the upstream error behind a stale result
}

// Get returns a fresh cached body or calls fetch. On fetch errors it falls back
// to the last good body (Stale=true) if there is one.
func (c *Cache) Get(key string, ttl time.Duration, now time.Time, fetch func() ([]byte, error)) (Result, error) {
	c.mu.Lock()
	l, ok := c.locks[key]
	if !ok {
		l = &sync.Mutex{}
		c.locks[key] = l
	}
	c.mu.Unlock()

	l.Lock()
	defer l.Unlock()

	c.mu.Lock()
	e, have := c.items[key]
	c.mu.Unlock()
	if have && now.Sub(e.fetched) < ttl {
		return Result{Body: e.body, Fetched: e.fetched}, nil
	}
	body, err := fetch()
	if err != nil {
		if have {
			return Result{Body: e.body, Fetched: e.fetched, Stale: true, Err: err}, nil
		}
		return Result{}, err
	}
	c.mu.Lock()
	c.items[key] = entry{body: body, fetched: now}
	c.mu.Unlock()
	return Result{Body: body, Fetched: now}, nil
}

// Budget counts upstream requests per local day and persists the count.
type Budget struct {
	mu    sync.Mutex
	path  string
	Limit int    `json:"-"`
	Day   string `json:"day"`
	Used  int    `json:"used"`
}

func NewBudget(dataDir string, limit int) *Budget {
	b := &Budget{Limit: limit}
	if dataDir != "" {
		b.path = filepath.Join(dataDir, "budget.json")
		if raw, err := os.ReadFile(b.path); err == nil {
			_ = json.Unmarshal(raw, b)
		}
	}
	return b
}

// Take reserves one request for the day of now.
func (b *Budget) Take(now time.Time) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	day := now.Format("2006-01-02")
	if b.Day != day {
		b.Day, b.Used = day, 0
	}
	if b.Limit > 0 && b.Used >= b.Limit {
		return ErrBudget
	}
	b.Used++
	b.save()
	return nil
}

func (b *Budget) Snapshot(now time.Time) (used, limit int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.Day != now.Format("2006-01-02") {
		return 0, b.Limit
	}
	return b.Used, b.Limit
}

func (b *Budget) save() {
	if b.path == "" {
		return
	}
	raw, _ := json.Marshal(b)
	tmp := b.path + ".tmp"
	if os.WriteFile(tmp, raw, 0o644) == nil {
		_ = os.Rename(tmp, b.path) // a failed write must not break the view
	}
}
