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

func TestTraceHandlerEventStartsTransportChildProcessSpan(t *testing.T) {
	restorePropagator := setTraceContextPropagator()
	defer restorePropagator()

	tp, exporter := newTestTracerProvider()
	propagator := otel.GetTextMapPropagator()
	upstreamTracer := tp.Tracer("upstream")
	transportTracer := tp.Tracer("transport")

	upstreamCtx, upstreamSpan := upstreamTracer.Start(context.Background(), "upstream-event")
	msg := message.NewMessage("1", nil)
	SetMessageKind(msg, MessageKindEvent)
	propagator.Inject(upstreamCtx, metadataWrapper{msg.Metadata})
	upstreamSpan.End()

	handler := TraceNoPublishHandler(func(msg *message.Message) error {
		return nil
	},
		WithTracer(transportTracer),
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
	processSpan := findSpanByNameContains(t, spans, "[event]")

	assert.Equal(t, transportSpan.SpanContext().TraceID(), processSpan.Parent().TraceID())
	assert.Equal(t, transportSpan.SpanContext().SpanID(), processSpan.Parent().SpanID())
	assert.Len(t, processSpan.Links(), 1)
	assert.ElementsMatch(t, []trace.SpanContext{
		transportSpan.SpanContext(),
	}, spanContextsFromLinks(processSpan.Links()))

	assert.Len(t, transportSpan.Links(), 1)
	assert.ElementsMatch(t, []trace.SpanContext{
		upstreamSpan.SpanContext().WithRemote(true),
	}, spanContextsFromLinks(transportSpan.Links()))
}

func TestTraceHandlerCommandResponseCreatesTransportAndWorkflowProcessSpans(t *testing.T) {
	restorePropagator := setTraceContextPropagator()
	defer restorePropagator()

	tp, exporter := newTestTracerProvider()
	propagator := otel.GetTextMapPropagator()
	upstreamTracer := tp.Tracer("upstream")
	transportTracer := tp.Tracer("transport")
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
		WithCommandResponseRegistry(registry),
	)

	err := handler(msg)
	if !assert.NoError(t, err) {
		return
	}

	spans := exporter.Spans()
	if !assert.Len(t, spans, 5) {
		return
	}

	transportSpan := findSpanByKind(t, spans, trace.SpanKindConsumer)
	processSpans := findSpansByNameContains(spans, "[command_response]")
	if !assert.Len(t, processSpans, 2) {
		return
	}

	var transportChildSpan sdktrace.ReadOnlySpan
	var workflowChildSpan sdktrace.ReadOnlySpan
	for _, span := range processSpans {
		if span.Parent().SpanID() == transportSpan.SpanContext().SpanID() {
			transportChildSpan = span
		}
		if span.Parent().SpanID() == workflowSpan.SpanContext().SpanID() {
			workflowChildSpan = span
		}
	}

	if assert.NotNil(t, transportChildSpan) {
		assert.Len(t, transportChildSpan.Links(), 1)
		assert.ElementsMatch(t, []trace.SpanContext{
			transportSpan.SpanContext(),
		}, spanContextsFromLinks(transportChildSpan.Links()))
	}

	if assert.NotNil(t, workflowChildSpan) {
		assert.Equal(t, workflowSpan.SpanContext().TraceID(), workflowChildSpan.Parent().TraceID())
		assert.Equal(t, workflowSpan.SpanContext().SpanID(), workflowChildSpan.Parent().SpanID())
		assert.Len(t, workflowChildSpan.Links(), 1)
		assert.ElementsMatch(t, []trace.SpanContext{
			transportChildSpan.SpanContext(),
		}, spanContextsFromLinks(workflowChildSpan.Links()))
	}

	assert.Len(t, transportSpan.Links(), 1)
	assert.ElementsMatch(t, []trace.SpanContext{
		calleeSpan.SpanContext().WithRemote(true),
	}, spanContextsFromLinks(transportSpan.Links()))

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

func findSpansByNameContains(spans []sdktrace.ReadOnlySpan, fragment string) []sdktrace.ReadOnlySpan {
	result := make([]sdktrace.ReadOnlySpan, 0)
	for _, span := range spans {
		if span.Name() != "" && strings.Contains(span.Name(), fragment) {
			result = append(result, span)
		}
	}

	return result
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
