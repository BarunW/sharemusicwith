package main

import (
	"context"
	"log"
	"mime"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"connect-with-playlist-wasm/internal/api"
	"connect-with-playlist-wasm/internal/config"
	"connect-with-playlist-wasm/internal/store"
)

func main() {
	cfg := config.Load()
	if cfg.DatabaseURL == "" {
		cfg.DatabaseURL = "postgresql://dbarun:mypassword@0.0.0.0:5432/playlists?sslmode=disable"
	}
	if cfg.DatabaseURL == "" {
		log.Fatal("DATABASE_URL is required (e.g. postgres://dbarun:mypassword@localhost:5432/playlists?sslmode=disable)")
	}
	if err := mime.AddExtensionType(".wasm", "application/wasm"); err != nil {
		log.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	startupCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	st, err := store.New(startupCtx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("connect to database: %v", err)
	}
	defer st.Close()
	if err := st.ApplySchema(startupCtx); err != nil {
		log.Fatalf("apply schema: %v", err)
	}

	if cfg.MetricsSalt == "" {
		log.Print("warning: METRICS_SALT is unset — metric events are stored but not counted toward rankings")
	}

	// Refresh the discovery rankings periodically from the (deduped, non-bot)
	// events. Runs once at boot so the feed isn't empty, then every minute;
	// stops when the signal context is cancelled.
	go func() {
		refresh := func() {
			rctx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			if err := st.RefreshRankings(rctx); err != nil {
				log.Printf("refresh rankings: %v", err)
			}
		}
		refresh()
		t := time.NewTicker(60 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				refresh()
			}
		}
	}()

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           api.NewRouter(st, cfg),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
	}

	go func() {
		log.Printf("serving %s at http://%s/", cfg.StaticDir, cfg.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	<-ctx.Done()
	log.Print("shutting down")
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}
