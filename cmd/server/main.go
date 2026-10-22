package main

import (
	"log"
	"net/http"

	"github.com/Mythicove/mythic-hoard/internal/hoard"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func main() {
	store, err := hoard.New("data/files")
	if err != nil {
		log.Fatalf("init store: %v", err)
	}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Logger)
	store.Register(r)

	log.Println("mythic-hoard listening on :8081")
	if err := http.ListenAndServe(":8081", r); err != nil {
		log.Fatal(err)
	}
}
