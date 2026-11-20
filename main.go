// Command commute serves a door-to-door commute view with live public transport data.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	_ "time/tzdata" // distroless images have no zoneinfo

	"commute/internal/api"
	"commute/internal/bike"
	"commute/internal/config"
	"commute/internal/store"
	"commute/internal/transit"
	"commute/internal/weather"
	"commute/web"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(healthcheck())
	}
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))

	cfgPath := os.Getenv("CONFIG_PATH")
	if cfgPath == "" {
		cfgPath = "config.yaml"
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Error("config", "err", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		log.Warn("data dir not writable; budget will not persist", "dir", cfg.DataDir, "err", err)
	}

	srv := &api.Server{
		Cfg:     &cfg,
		Transit: transit.NewSearchCh(),
		Weather: weather.NewClient(),
		Bikes:   bike.NewStore(cfg.DataDir),
		Cache:   store.NewCache(),
		Budget:  store.NewBudget(cfg.DataDir, cfg.Transit.DailyRouteBudget),
		Web:     web.FS(),
		Now:     time.Now,
		Log:     log,
	}
	hs := &http.Server{Addr: cfg.Listen, Handler: srv.Handler(), ReadHeaderTimeout: 5 * time.Second}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shut, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = hs.Shutdown(shut)
	}()
	log.Info("listening", "addr", cfg.Listen, "config", cfgPath, "data", cfg.DataDir)
	if err := hs.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("server", "err", err)
		os.Exit(1)
	}
}

// healthcheck lets the distroless image check itself without curl.
func healthcheck() int {
	addr := os.Getenv("LISTEN")
	if addr == "" {
		addr = ":8080"
	}
	if strings.HasPrefix(addr, ":") {
		addr = "127.0.0.1" + addr
	}
	c := http.Client{Timeout: 3 * time.Second}
	resp, err := c.Get("http://" + addr + "/healthz")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}
