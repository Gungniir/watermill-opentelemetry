package opentelemetry

import (
	"sync"

	"go.opentelemetry.io/otel/trace"
)

type CommandResponseRegistry interface {
	Save(correlationID string, spanContext trace.SpanContext)
	Load(correlationID string) (trace.SpanContext, bool)
	Delete(correlationID string)
}

type InMemoryCommandResponseRegistry struct {
	mu    sync.RWMutex
	items map[string]trace.SpanContext
}

func NewInMemoryCommandResponseRegistry() *InMemoryCommandResponseRegistry {
	return &InMemoryCommandResponseRegistry{
		items: map[string]trace.SpanContext{},
	}
}

func (r *InMemoryCommandResponseRegistry) Save(correlationID string, spanContext trace.SpanContext) {
	if correlationID == "" || !spanContext.IsValid() {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.items[correlationID] = spanContext
}

func (r *InMemoryCommandResponseRegistry) Load(correlationID string) (trace.SpanContext, bool) {
	if correlationID == "" {
		return trace.SpanContext{}, false
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	spanContext, ok := r.items[correlationID]

	return spanContext, ok
}

func (r *InMemoryCommandResponseRegistry) Delete(correlationID string) {
	if correlationID == "" {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.items, correlationID)
}

var defaultCommandResponseRegistry CommandResponseRegistry = NewInMemoryCommandResponseRegistry()
