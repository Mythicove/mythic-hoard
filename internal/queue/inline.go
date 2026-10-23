// Package queue 提供任务队列实现（内联同步 / asynq Redis）。
package queue

import (
	"context"

	"github.com/Mythicove/shared/queue"
)

// Inline 是内联同步执行的队列（低档）。
type Inline struct {
	handlers map[string]queue.Handler
}

var _ queue.Queue = (*Inline)(nil)

// NewInline 构造内联队列。
func NewInline() *Inline {
	return &Inline{handlers: map[string]queue.Handler{}}
}

func (q *Inline) Enqueue(ctx context.Context, t queue.Task) error {
	if h, ok := q.handlers[t.Type]; ok {
		return h(ctx, t)
	}
	return nil
}

func (q *Inline) Register(taskType string, h queue.Handler) {
	q.handlers[taskType] = h
}
