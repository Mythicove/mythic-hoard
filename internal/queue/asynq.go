package queue

import (
	"context"
	"encoding/json"
	"log"

	"github.com/Mythicove/shared/queue"
	"github.com/hibiken/asynq"
)

// Asynq 是基于 Redis 的异步队列（高档）。
type Asynq struct {
	client *asynq.Client
	server *asynq.Server
	mux    *asynq.ServeMux
}

var _ queue.Queue = (*Asynq)(nil)

// NewAsynq 构造 asynq 队列并启动消费。
func NewAsynq(addr string) (*Asynq, error) {
	client := asynq.NewClient(asynq.RedisClientOpt{Addr: addr})
	server := asynq.NewServer(asynq.RedisClientOpt{Addr: addr}, asynq.Config{
		Concurrency: 4,
	})
	mux := asynq.NewServeMux()
	q := &Asynq{client: client, server: server, mux: mux}
	go func() {
		if err := server.Run(mux); err != nil {
			log.Printf("asynq server: %v", err)
		}
	}()
	return q, nil
}

func (q *Asynq) Enqueue(_ context.Context, t queue.Task) error {
	payload, err := json.Marshal(t.Payload)
	if err != nil {
		return err
	}
	_, err = q.client.Enqueue(asynq.NewTask(t.Type, payload))
	return err
}

func (q *Asynq) Register(taskType string, h queue.Handler) {
	q.mux.HandleFunc(taskType, func(ctx context.Context, task *asynq.Task) error {
		var payload json.RawMessage
		_ = json.Unmarshal(task.Payload(), &payload)
		return h(ctx, queue.Task{Type: taskType, Payload: payload})
	})
}
