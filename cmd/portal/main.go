// Command portal runs voxin.id: the public news site and the newsroom CMS,
// on two listeners so the CMS never shows up on the public domain.
package main

import (
	"bufio"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/hilmimuharromi/voxin-portal/internal/cms"
	"github.com/hilmimuharromi/voxin-portal/internal/config"
	"github.com/hilmimuharromi/voxin-portal/internal/core"
	"github.com/hilmimuharromi/voxin-portal/internal/site"
	"github.com/hilmimuharromi/voxin-portal/internal/store"
)

// loadDotEnv reads KEY=VALUE lines from .env without overriding real env vars.
func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		l := strings.TrimSpace(sc.Text())
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		k, v, ok := strings.Cut(l, "=")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.Trim(strings.TrimSpace(v), `"'`)
		if _, set := os.LookupEnv(k); !set {
			os.Setenv(k, v)
		}
	}
}

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	loadDotEnv(".env")
	cfg, err := config.Load()
	if err != nil {
		log.Error("berhenti", "err", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	st, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("berhenti", "err", err)
		os.Exit(1)
	}
	app := core.New(st, cfg, log)
	pub, err := site.New(app)
	if err != nil {
		log.Error("template situs", "err", err)
		os.Exit(1)
	}
	back, err := cms.New(app)
	if err != nil {
		log.Error("template CMS", "err", err)
		os.Exit(1)
	}
	if err := back.EnsureAdmin(ctx); err != nil {
		log.Error("berhenti", "err", err)
		os.Exit(1)
	}
	go app.Run(ctx)

	servers := []*http.Server{
		{Addr: cfg.PublicAddr, Handler: pub.Handler(), ReadHeaderTimeout: 10 * time.Second, WriteTimeout: 30 * time.Second},
		{Addr: cfg.CMSAddr, Handler: back.Handler(), ReadHeaderTimeout: 10 * time.Second, WriteTimeout: 120 * time.Second},
	}
	errc := make(chan error, 2)
	for _, srv := range servers {
		go func(s *http.Server) {
			if err := s.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				errc <- err
			}
		}(srv)
	}
	log.Info("voxin.id jalan", "situs", cfg.BaseURL, "listen_situs", cfg.PublicAddr, "cms", cfg.CMSURL, "listen_cms", cfg.CMSAddr)
	select {
	case <-ctx.Done():
	case err := <-errc:
		log.Error("server berhenti", "err", err)
	}
	sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, s := range servers {
		_ = s.Shutdown(sctx)
	}
}
