package main

import (
	"log"
	"net/http"

	"github.com/Mythicove/mythic-hoard/internal/config"
	"github.com/Mythicove/mythic-hoard/internal/hoard"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func main() {
	cfg := config.Load()

	store, err := hoard.New(cfg.DataDir)
	if err != nil {
		log.Fatalf("init store: %v", err)
	}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Logger)
	store.Register(r, cfg.AdminToken)

	log.Printf("mythic-hoard listening on %s", cfg.ListenAddr)
	if err := http.ListenAndServe(cfg.ListenAddr, r); err != nil {
		log.Fatal(err)
	}
}
