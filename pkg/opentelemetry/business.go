package opentelemetry

import (
	"context"
	"fmt"

	"github.com/ThreeDotsLabs/watermill/message"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	wotelmeta "github.com/Gungniir/watermill-opentelemetry/pkg/opentelemetry/metadata"
)

const businessTracerName = "watermill/subscriber/business"

const (
	attrMessageKind        = attribute.Key("messaging.message.kind")
	attrMissingContext     = attribute.Key("messaging.message.context_missing")
	attrRegistryMiss       = attribute.Key("messaging.message.command_response_registry_miss")
	attrInvalidMessageKind = attribute.Key("messaging.message.invalid_kind")
)

func defaultBusinessSpanName(handler, topic, kind string) string {
	return fmt.Sprintf("%s: %s [%s]", handler, topic, kind)
}

func startBusinessSpan(config *config, msg *message.Message, transportCtx context.Context, extractedCtx context.Context) (context.Context, trace.Span) {
	msgctx := msg.Context()
	handlerName := message.HandlerNameFromCtx(msgctx)
	topic := message.SubscribeTopicFromCtx(msgctx)
	kind := messageKind(msg, config)
	transportSpanContext := trace.SpanContextFromContext(transportCtx)
	upstreamSpanContext := trace.SpanContextFromContext(extractedCtx)

	spanNameFunc := config.businessSpanNameFunc
	if spanNameFunc == nil {
		spanNameFunc = defaultBusinessSpanName
	}

	tracer := config.businessTracer
	if tracer == nil {
		tracer = otel.Tracer(businessTracerName)
	}

	attrs := []attribute.KeyValue{
		attrMessageKind.String(kind),
	}

	var (
		ctx  context.Context
		span trace.Span
	)

	switch kind {
	case MessageKindCommand:
		opts := []trace.SpanStartOption{
			trace.WithAttributes(attrs...),
		}

		if transportSpanContext.IsValid() {
			opts = append(opts, trace.WithLinks(trace.Link{SpanContext: transportSpanContext}))
		}

		if upstreamSpanContext.IsValid() {
			ctx, span = tracer.Start(extractedCtx, spanNameFunc(handlerName, topic, kind), opts...)
		} else {
			attrs = append(attrs, attrMissingContext.Bool(true))
			opts[0] = trace.WithAttributes(attrs...)
			opts = append(opts, trace.WithNewRoot())
			ctx, span = tracer.Start(msgctx, spanNameFunc(handlerName, topic, kind), opts...)
		}

	case MessageKindCommandResponse:
		opts := []trace.SpanStartOption{
			trace.WithAttributes(attrs...),
		}
		links := make([]trace.Link, 0, 2)
		if upstreamSpanContext.IsValid() {
			links = append(links, trace.Link{SpanContext: upstreamSpanContext})
		}
		if transportSpanContext.IsValid() {
			links = append(links, trace.Link{SpanContext: transportSpanContext})
		}
		if len(links) > 0 {
			opts = append(opts, trace.WithLinks(links...))
		}

		correlationID := msg.Metadata.Get(wotelmeta.CorrelationID)
		if originalSpanContext, ok := config.commandResponseRegistry.Load(correlationID); ok && originalSpanContext.IsValid() {
			parentCtx := trace.ContextWithSpanContext(msgctx, originalSpanContext)
			ctx, span = tracer.Start(parentCtx, spanNameFunc(handlerName, topic, kind), opts...)
		} else {
			attrs = append(attrs, attrRegistryMiss.Bool(true))
			opts[0] = trace.WithAttributes(attrs...)
			opts = append(opts, trace.WithNewRoot())
			ctx, span = tracer.Start(msgctx, spanNameFunc(handlerName, topic, kind), opts...)
		}

	case "", MessageKindEvent:
		if kind == "" {
			kind = MessageKindEvent
			attrs[0] = attrMessageKind.String(kind)
		}

		opts := []trace.SpanStartOption{
			trace.WithAttributes(attrs...),
			trace.WithNewRoot(),
		}
		links := make([]trace.Link, 0, 2)
		if upstreamSpanContext.IsValid() {
			links = append(links, trace.Link{SpanContext: upstreamSpanContext})
		}
		if transportSpanContext.IsValid() {
			links = append(links, trace.Link{SpanContext: transportSpanContext})
		}
		if len(links) > 0 {
			opts = append(opts, trace.WithLinks(links...))
		}

		ctx, span = tracer.Start(msgctx, spanNameFunc(handlerName, topic, kind), opts...)

	default:
		attrs = append(attrs, attrInvalidMessageKind.String(kind))
		opts := []trace.SpanStartOption{
			trace.WithAttributes(attrs...),
			trace.WithNewRoot(),
		}
		links := make([]trace.Link, 0, 2)
		if transportSpanContext.IsValid() {
			links = append(links, trace.Link{SpanContext: transportSpanContext})
		}
		if upstreamSpanContext.IsValid() {
			links = append(links, trace.Link{SpanContext: upstreamSpanContext})
		}
		if len(links) > 0 {
			opts = append(opts, trace.WithLinks(links...))
		}
		ctx, span = tracer.Start(msgctx, spanNameFunc(handlerName, topic, kind), opts...)
		span.SetStatus(codes.Error, "unsupported message_kind")
	}

	return ctx, span
}
