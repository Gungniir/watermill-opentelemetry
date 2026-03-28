package opentelemetry

import (
	"context"

	"github.com/ThreeDotsLabs/watermill/message"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	wotelmeta "github.com/Gungniir/watermill-opentelemetry/pkg/opentelemetry/metadata"
)

const (
	attrMessageKind = attribute.Key("messaging.message.kind")
)

func startProcessSpan(config *config, msg *message.Message, consumingCtx context.Context, upstreamCtx context.Context) (ctx context.Context, processSpan trace.Span, linkedProcessSpan trace.Span) {
	msgctx := msg.Context()
	handlerName := message.HandlerNameFromCtx(msgctx)
	topic := message.SubscribeTopicFromCtx(msgctx)
	kind := messageKind(msg, config)
	consumingSpanContext := trace.SpanContextFromContext(consumingCtx)
	upstreamSpanContext := trace.SpanContextFromContext(upstreamCtx)

	attrs := []attribute.KeyValue{
		attrMessageKind.String(kind),
	}

	opts := []trace.SpanStartOption{
		trace.WithAttributes(attrs...),
	}

	if consumingSpanContext.IsValid() {
		opts = append(opts, trace.WithLinks(trace.Link{SpanContext: consumingSpanContext}))
	}

	ctx, processSpan = config.tracer.Start(consumingCtx, config.processSpanNameFunc(handlerName, topic, kind), opts...)

	switch kind {
	case MessageKindCommand:
		if upstreamSpanContext.IsValid() {
			opts := []trace.SpanStartOption{
				trace.WithAttributes(attrs...),
				trace.WithLinks(trace.LinkFromContext(ctx)),
			}

			ctx, linkedProcessSpan = config.tracer.Start(upstreamCtx, config.processSpanNameFunc(handlerName, topic, kind), opts...)
		}
	case MessageKindCommandResponse:
		correlationID := msg.Metadata.Get(wotelmeta.CorrelationID)
		if originalSpanContext, ok := config.commandResponseRegistry.Load(correlationID); ok && originalSpanContext.IsValid() {
			opts := []trace.SpanStartOption{
				trace.WithAttributes(attrs...),
				trace.WithLinks(trace.LinkFromContext(ctx)),
			}

			parentCtx := trace.ContextWithSpanContext(msgctx, originalSpanContext)
			ctx, linkedProcessSpan = config.tracer.Start(parentCtx, config.processSpanNameFunc(handlerName, topic, kind), opts...)
		}
	}

	return ctx, processSpan, linkedProcessSpan
}
