package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"murad.world/murad-world/internal/baserow"
	"murad.world/murad-world/internal/config"
	"murad.world/murad-world/internal/content"
	"murad.world/murad-world/internal/web"
)

func main() {
	log.SetPrefix("skull-black: ")
	log.SetFlags(log.LstdFlags | log.LUTC)

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	br := baserow.NewClient(cfg.BaserowAPIURL, cfg.BaserowTableID, cfg.BaserowToken)
	store := content.NewStore(br)
	if err := store.Start(ctx); err != nil {
		log.Fatalf("content: %v", err)
	}

	tmpl, err := web.LoadTemplates("templates")
	if err != nil {
		log.Fatalf("templates: %v", err)
	}

	s := web.NewServer(cfg, store, tmpl)
	mux := http.NewServeMux()
	s.Register(mux)

	httpServer := &http.Server{
		Addr:              ":" + strings.TrimSpace(cfg.Port),
		Handler:           securityHeadersMiddleware(mux),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		log.Printf("listening on :%s", cfg.Port)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http: %v", err)
		}
	}()

	<-ctx.Done()
	log.Printf("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("http shutdown: %v", err)
	}
}

func securityHeadersMiddleware(next http.Handler) http.Handler {
	const csp = "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data: https: http:; font-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'"
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Content-Security-Policy", csp)
		next.ServeHTTP(w, r)
	})
}
