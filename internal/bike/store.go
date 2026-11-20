package bike

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Store fetches each route once from BRouter and keeps the raw GeoJSON in the
// data directory, so the router is only called again when places or profile change.
type Store struct {
	DataDir string
	HTTP    *http.Client

	mu      sync.Mutex
	routes  map[string]*Route
	failed  map[string]time.Time
	lastErr map[string]error
}

func NewStore(dataDir string) *Store {
	return &Store{
		DataDir: dataDir,
		HTTP:    &http.Client{Timeout: 30 * time.Second},
		routes:  map[string]*Route{},
		failed:  map[string]time.Time{},
		lastErr: map[string]error{},
	}
}

const retryAfter = 10 * time.Minute

// Get returns the timed route from (fromLat, fromLon) to (toLat, toLon).
func (s *Store) Get(ctx context.Context, profile string, m Model, fromLat, fromLon, toLat, toLon float64, now time.Time) (*Route, error) {
	key := routeKey(profile, fromLat, fromLon, toLat, toLon)
	mkey := fmt.Sprintf("%s|%+v", key, m)

	s.mu.Lock()
	defer s.mu.Unlock()
	if r, ok := s.routes[mkey]; ok {
		return r, nil
	}
	path := ""
	if s.DataDir != "" {
		path = filepath.Join(s.DataDir, "bike_route_"+key+".geojson")
		if body, err := os.ReadFile(path); err == nil {
			if r, err := Parse(body, m); err == nil {
				s.routes[mkey] = r
				return r, nil
			}
		}
	}
	if t, ok := s.failed[key]; ok && now.Sub(t) < retryAfter {
		return nil, s.lastErr[key]
	}
	body, err := Fetch(ctx, s.HTTP, profile, fromLat, fromLon, toLat, toLon)
	if err == nil {
		var r *Route
		if r, err = Parse(body, m); err == nil {
			if path != "" {
				_ = os.WriteFile(path, body, 0o644) // best effort; refetch next start otherwise
			}
			s.routes[mkey] = r
			delete(s.failed, key)
			return r, nil
		}
	}
	s.failed[key], s.lastErr[key] = now, err
	return nil, err
}

func routeKey(profile string, a, b, c, d float64) string {
	h := sha1.Sum([]byte(fmt.Sprintf("%s|%.5f,%.5f|%.5f,%.5f", profile, a, b, c, d)))
	return hex.EncodeToString(h[:])[:12]
}
