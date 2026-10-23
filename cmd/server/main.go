package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Mythicove/mythic-hoard/internal/config"
	"github.com/Mythicove/mythic-hoard/internal/hoard"
	"github.com/Mythicove/mythic-hoard/internal/migrate"
	"github.com/Mythicove/mythic-hoard/internal/pgstore"
	queueimpl "github.com/Mythicove/mythic-hoard/internal/queue"
	"github.com/Mythicove/mythic-hoard/internal/search"
	hoardstorage "github.com/Mythicove/mythic-hoard/internal/storage"
	"github.com/Mythicove/shared/queue"
	"github.com/Mythicove/shared/storage"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"golang.org/x/time/rate"
)

func main() {
	cfg := config.Load()

	if cfg.DatabaseURL != "" {
		if err := migrate.Run(cfg.DatabaseURL); err != nil {
			log.Fatalf("migrate: %v", err)
		}
	}

	files, err := buildStorage(context.Background(), cfg)
	if err != nil {
		log.Fatalf("init storage: %v", err)
	}

	store, closeStore, err := newStore(cfg)
	if err != nil {
		log.Fatalf("init store: %v", err)
	}
	if closeStore != nil {
		defer closeStore()
	}

	svc := hoard.NewService(store, files, cfg.BaseURL)
	if cfg.SearchDriver == "meili" && cfg.MeiliURL != "" {
		svc.SetSearch(search.NewMeili(cfg.MeiliURL, cfg.MeiliAPIKey))
	}
	svc.SetQueue(buildQueue(cfg))

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Logger)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: false,
		MaxAge:           300,
	}))
	if cfg.RateLimitRPS > 0 {
		r.Use(hoard.RateLimit(rate.Limit(cfg.RateLimitRPS), cfg.RateLimitBurst))
	}
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	svc.Register(r, cfg.AdminToken)

	srv := &http.Server{Addr: cfg.ListenAddr, Handler: r}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	log.Printf("mythic-hoard listening on %s", cfg.ListenAddr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server: %v", err)
	}
}

func buildStorage(ctx context.Context, cfg *config.Config) (storage.Storage, error) {
	if cfg.StorageDriver == "s3" {
		return hoardstorage.NewS3(ctx, hoardstorage.Options{
			Endpoint:  cfg.S3Endpoint,
			Region:    cfg.S3Region,
			Bucket:    cfg.S3Bucket,
			AccessKey: cfg.S3AccessKey,
			SecretKey: cfg.S3SecretKey,
		}, cfg.BaseURL+"/cdn")
	}
	return hoardstorage.NewLocal(cfg.DataDir, cfg.BaseURL+"/cdn")
}

func buildQueue(cfg *config.Config) queue.Queue {
	if cfg.QueueDriver == "asynq" && cfg.RedisAddr != "" {
		q, err := queueimpl.NewAsynq(cfg.RedisAddr)
		if err != nil {
			log.Printf("asynq: %v; falling back to inline", err)
			return queueimpl.NewInline()
		}
		return q
	}
	return queueimpl.NewInline()
}

// newStore 有数据库 URL 用 Postgres，否则退回内存（开发/测试）。
func newStore(cfg *config.Config) (hoard.Store, func(), error) {
	if cfg.DatabaseURL == "" {
		return hoard.NewMemoryStore(), nil, nil
	}
	pg, err := pgstore.New(context.Background(), cfg.DatabaseURL)
	if err != nil {
		return nil, nil, err
	}
	return pg, pg.Close, nil
}
