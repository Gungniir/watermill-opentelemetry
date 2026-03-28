package opentelemetry

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"

	wotelmeta "github.com/Gungniir/watermill-opentelemetry/pkg/opentelemetry/metadata"
)

type spanExporter struct {
	mu    sync.Mutex
	spans []sdktrace.ReadOnlySpan
}

func (e *spanExporter) ExportSpans(_ context.Context, spans []sdktrace.ReadOnlySpan) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.spans = append(e.spans, spans...)

	return nil
}

func (e *spanExporter) Shutdown(context.Context) error {
	return nil
}

func (e *spanExporter) Spans() []sdktrace.ReadOnlySpan {
	e.mu.Lock()
	defer e.mu.Unlock()

	out := make([]sdktrace.ReadOnlySpan, len(e.spans))
	copy(out, e.spans)

	return out
}

func TestTraceHandlerEventStartsNewBusinessRootWithLinks(t *testing.T) {
	restorePropagator := setTraceContextPropagator()
	defer restorePropagator()

	tp, exporter := newTestTracerProvider()
	propagator := otel.GetTextMapPropagator()
	upstreamTracer := tp.Tracer("upstream")
	transportTracer := tp.Tracer("transport")
	businessTracer := tp.Tracer("business")

	upstreamCtx, upstreamSpan := upstreamTracer.Start(context.Background(), "upstream-event")
	msg := message.NewMessage("1", nil)
	SetMessageKind(msg, MessageKindEvent)
	propagator.Inject(upstreamCtx, metadataWrapper{msg.Metadata})
	upstreamSpan.End()

	handler := TraceNoPublishHandler(func(msg *message.Message) error {
		return nil
	},
		WithTracer(transportTracer),
		WithBusinessTracer(businessTracer),
	)

	err := handler(msg)
	if !assert.NoError(t, err) {
		return
	}

	spans := exporter.Spans()
	if !assert.Len(t, spans, 3) {
		return
	}

	transportSpan := findSpanByKind(t, spans, trace.SpanKindConsumer)
	businessSpan := findSpanByNameContains(t, spans, "[event]")

	assert.False(t, businessSpan.Parent().IsValid())
	assert.Len(t, businessSpan.Links(), 2)
	assert.ElementsMatch(t, []trace.SpanContext{
		upstreamSpan.SpanContext().WithRemote(true),
		transportSpan.SpanContext(),
	}, spanContextsFromLinks(businessSpan.Links()))
}

func TestTraceHandlerCommandResponseResumesOriginalWorkflowContext(t *testing.T) {
	restorePropagator := setTraceContextPropagator()
	defer restorePropagator()

	tp, exporter := newTestTracerProvider()
	propagator := otel.GetTextMapPropagator()
	upstreamTracer := tp.Tracer("upstream")
	transportTracer := tp.Tracer("transport")
	businessTracer := tp.Tracer("business")
	registry := NewInMemoryCommandResponseRegistry()

	_, workflowSpan := upstreamTracer.Start(context.Background(), "workflow-root")
	calleeCtx, calleeSpan := upstreamTracer.Start(context.Background(), "callee-reply")

	msg := message.NewMessage("1", nil)
	SetMessageKind(msg, MessageKindCommandResponse)
	msg.Metadata.Set(wotelmeta.CorrelationID, "corr-1")
	propagator.Inject(calleeCtx, metadataWrapper{msg.Metadata})
	registry.Save("corr-1", workflowSpan.SpanContext())
	calleeSpan.End()
	workflowSpan.End()

	handler := TraceNoPublishHandler(func(msg *message.Message) error {
		return nil
	},
		WithTracer(transportTracer),
		WithBusinessTracer(businessTracer),
		WithCommandResponseRegistry(registry),
	)

	err := handler(msg)
	if !assert.NoError(t, err) {
		return
	}

	spans := exporter.Spans()
	if !assert.Len(t, spans, 4) {
		return
	}

	transportSpan := findSpanByKind(t, spans, trace.SpanKindConsumer)
	businessSpan := findSpanByNameContains(t, spans, "[command_response]")

	assert.Equal(t, workflowSpan.SpanContext().TraceID(), businessSpan.Parent().TraceID())
	assert.Equal(t, workflowSpan.SpanContext().SpanID(), businessSpan.Parent().SpanID())
	assert.Len(t, businessSpan.Links(), 2)
	assert.ElementsMatch(t, []trace.SpanContext{
		calleeSpan.SpanContext().WithRemote(true),
		transportSpan.SpanContext(),
	}, spanContextsFromLinks(businessSpan.Links()))

	_, ok := registry.Load("corr-1")
	assert.False(t, ok)
}

func newTestTracerProvider() (*sdktrace.TracerProvider, *spanExporter) {
	exporter := &spanExporter{}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exporter),
		sdktrace.WithResource(resource.Empty()),
	)

	return tp, exporter
}

func findSpanByKind(t *testing.T, spans []sdktrace.ReadOnlySpan, kind trace.SpanKind) sdktrace.ReadOnlySpan {
	t.Helper()

	for _, span := range spans {
		if span.SpanKind() == kind {
			return span
		}
	}

	t.Fatalf("span with kind %s not found", kind)

	return nil
}

func findSpanByNameContains(t *testing.T, spans []sdktrace.ReadOnlySpan, fragment string) sdktrace.ReadOnlySpan {
	t.Helper()

	for _, span := range spans {
		if span.Name() != "" && strings.Contains(span.Name(), fragment) {
			return span
		}
	}

	t.Fatalf("span containing %q not found", fragment)

	return nil
}
func spanContextsFromLinks(links []sdktrace.Link) []trace.SpanContext {
	result := make([]trace.SpanContext, 0, len(links))
	for _, link := range links {
		result = append(result, link.SpanContext)
	}

	return result
}

func setTraceContextPropagator() func() {
	prev := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(propagation.TraceContext{})

	return func() {
		otel.SetTextMapPropagator(prev)
	}
}
