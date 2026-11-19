// Package api serves the JSON API and the embedded PWA.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"commute/internal/config"
	"commute/internal/engine"
	"commute/internal/store"
	"commute/internal/transit"
	"commute/internal/weather"
)

type Server struct {
	Cfg     *config.Config
	Transit *transit.SearchCh
	Weather *weather.Client
	Cache   *store.Cache
	Budget  *store.Budget
	Web     fs.FS // built PWA (may be empty)
	Now     func() time.Time
	Log     *slog.Logger
}

type budgetInfo struct {
	Used  int `json:"used"`
	Limit int `json:"limit"`
}

type LiveResponse struct {
	Now         time.Time            `json:"now"`
	Plan        engine.Plan          `json:"plan"`
	From        string               `json:"from"`
	To          string               `json:"to"`
	HeroID      string               `json:"hero_id"`
	Connections []transit.Connection `json:"connections"`
	Source      string               `json:"source"`
	FetchedAt   time.Time            `json:"fetched_at"`
	Stale       bool                 `json:"stale"`
	Warning     string               `json:"warning,omitempty"`
	Budget      budgetInfo           `json:"budget"`
	WeatherInfo weatherInfo          `json:"weather"`
}

type weatherInfo struct {
	Status  string    `json:"status"` // ok | stale | unavailable
	Source  string    `json:"source,omitempty"`
	Fetched time.Time `json:"fetched,omitzero"`
	Error   string    `json:"error,omitempty"`
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/live", s.live)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })
	mux.Handle("/", s.static())
	return mux
}

func (s *Server) live(w http.ResponseWriter, r *http.Request) {
	now := s.Now().In(s.Cfg.Loc)
	ov := engine.Override{Now: r.URL.Query().Get("when") == "now"}
	switch d := engine.Direction(r.URL.Query().Get("dir")); d {
	case engine.ToWork, engine.ToHome:
		ov.Direction = d
	}
	plan := engine.Decide(now, s.Cfg, ov)

	from, to := s.Cfg.Places.Home, s.Cfg.Places.Work
	if plan.Direction == engine.ToHome {
		from, to = to, from
	}
	q := transit.Query{FromLat: from.Lat, FromLon: from.Lon, ToLat: to.Lat, ToLon: to.Lon, At: plan.At, Num: s.Cfg.Transit.Num}

	key := string(plan.Direction) + "|live"
	ttl := time.Duration(s.Cfg.Transit.LiveTTLSeconds) * time.Second
	if plan.Mode == engine.Planned {
		key = string(plan.Direction) + "|" + plan.At.Format(time.RFC3339)
		ttl = time.Duration(s.Cfg.Transit.PlannedTTLSeconds) * time.Second
	}

	res, err := s.Cache.Get(key, ttl, now, func() ([]byte, error) {
		if err := s.Budget.Take(now); err != nil {
			return nil, err
		}
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		s.Log.Info("search.ch request", "dir", plan.Direction, "mode", plan.Mode)
		return s.Transit.Fetch(ctx, q, s.Cfg.Loc)
	})
	if err != nil {
		s.Log.Error("transit fetch failed", "err", err)
		msg := "Timetable unavailable: " + err.Error()
		if errors.Is(err, store.ErrBudget) {
			msg = "Daily search.ch budget used up. Connections return tomorrow."
		}
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": msg, "plan": plan})
		return
	}

	cs, err := transit.Parse(res.Body, transit.NormaliseOpts{
		Loc: s.Cfg.Loc, BufferMin: s.Cfg.BufferMin, WalkOverrides: s.Cfg.Transit.WalkOverrides,
	})
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error(), "plan": plan})
		return
	}
	hero, list := engine.Pick(now, plan, cs)

	wx, wxInfo := s.weatherData(r.Context(), now)
	if wx != nil {
		origin, dest := 0, 1
		if plan.Direction == engine.ToHome {
			origin, dest = 1, 0
		}
		for i := range list {
			list[i].Weather = weather.Assess(list[i], wx, origin, dest)
		}
	}

	used, limit := s.Budget.Snapshot(now)
	resp := LiveResponse{
		Now: now, Plan: plan, From: from.Label, To: to.Label,
		HeroID: hero, Connections: list,
		Source: "search.ch", FetchedAt: res.Fetched, Stale: res.Stale,
		Budget:      budgetInfo{Used: used, Limit: limit},
		WeatherInfo: wxInfo,
	}
	if list == nil {
		resp.Connections = []transit.Connection{}
	}
	if res.Stale {
		resp.Warning = "Showing the last good timetable from " + res.Fetched.In(s.Cfg.Loc).Format("15:04")
		if errors.Is(res.Err, store.ErrBudget) {
			resp.Warning += " (daily search.ch budget used up)"
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// weatherData returns the parsed forecast for home and work. Forecast and ensemble are
// cached separately; the ensemble is optional, the forecast is required.
func (s *Server) weatherData(ctx context.Context, now time.Time) (*weather.Data, weatherInfo) {
	if s.Weather == nil {
		return nil, weatherInfo{Status: "unavailable", Error: "weather disabled"}
	}
	pts := []weather.Coord{
		{Lat: s.Cfg.Places.Home.Lat, Lon: s.Cfg.Places.Home.Lon},
		{Lat: s.Cfg.Places.Work.Lat, Lon: s.Cfg.Places.Work.Lon},
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	type out struct {
		res store.Result
		err error
	}
	fc, ens := make(chan out, 1), make(chan out, 1)
	go func() {
		r, err := s.Cache.Get("wx|forecast", 30*time.Minute, now, func() ([]byte, error) {
			s.Log.Info("open-meteo forecast request")
			return s.Weather.FetchForecast(ctx, pts)
		})
		fc <- out{r, err}
	}()
	go func() {
		r, err := s.Cache.Get("wx|ensemble", 60*time.Minute, now, func() ([]byte, error) {
			s.Log.Info("open-meteo ensemble request")
			return s.Weather.FetchEnsemble(ctx, pts)
		})
		ens <- out{r, err}
	}()
	f, e := <-fc, <-ens
	if f.err != nil {
		s.Log.Error("weather fetch failed", "err", f.err)
		return nil, weatherInfo{Status: "unavailable", Error: f.err.Error()}
	}
	var ensBody []byte
	if e.err == nil {
		ensBody = e.res.Body
	} else {
		s.Log.Warn("ensemble unavailable, using deterministic forecast", "err", e.err)
	}
	d, err := weather.Attach(f.res.Body, ensBody, pts)
	if err != nil {
		s.Log.Error("weather parse failed", "err", err)
		return nil, weatherInfo{Status: "unavailable", Error: err.Error()}
	}
	info := weatherInfo{Status: "ok", Source: d.Source, Fetched: f.res.Fetched}
	if f.res.Stale || e.res.Stale {
		info.Status = "stale"
	}
	return d, info
}

// static serves the embedded PWA with an index.html fallback for client routes.
func (s *Server) static() http.Handler {
	files := http.FileServerFS(s.Web)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" {
			p = "index.html"
		}
		if _, err := fs.Stat(s.Web, p); err != nil {
			if _, err := fs.Stat(s.Web, "index.html"); err != nil {
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				w.Write([]byte("Frontend not built. Run `make web` or use the Docker image. API: /api/live\n"))
				return
			}
			r.URL.Path = "/"
		}
		switch {
		case p == "sw.js" || p == "index.html" || strings.HasSuffix(p, ".webmanifest"):
			w.Header().Set("Cache-Control", "no-cache")
		case strings.HasPrefix(p, "assets/"):
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		files.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}
