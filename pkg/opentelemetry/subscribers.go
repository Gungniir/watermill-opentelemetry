package opentelemetry

import (
	"fmt"

	"github.com/ThreeDotsLabs/watermill/message"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.10.0"
	"go.opentelemetry.io/otel/trace"

	wotelmeta "github.com/Gungniir/watermill-opentelemetry/pkg/opentelemetry/metadata"
)

const subscriberTracerName = "watermill/subscriber"

// Trace defines a middleware that will add tracing.
func Trace(options ...Option) message.HandlerMiddleware {
	return func(h message.HandlerFunc) message.HandlerFunc {
		return TraceHandler(h, options...)
	}
}

// TraceHandler decorates a watermill HandlerFunc to add tracing when a message is received.
func TraceHandler(h message.HandlerFunc, options ...Option) message.HandlerFunc {
	config := &config{
		tracer: otel.Tracer(subscriberTracerName),
		spanNameFunc: func(handler, topic string) string {
			return fmt.Sprintf("%s: %s", handler, topic)
		},
		businessTracer:          otel.Tracer(businessTracerName),
		businessSpanNameFunc:    defaultBusinessSpanName,
		commandResponseRegistry: defaultCommandResponseRegistry,
	}

	for _, opt := range options {
		opt(config)
	}

	return func(msg *message.Message) ([]*message.Message, error) {
		msgctx := msg.Context()
		spanName := config.spanNameFunc(message.HandlerNameFromCtx(msgctx), message.SubscribeTopicFromCtx(msgctx))
		ctxWithParentSpan := getPropagator(config).Extract(msgctx, metadataWrapper{msg.Metadata})

		ctx, span := config.tracer.Start(ctxWithParentSpan, spanName,
			trace.WithSpanKind(trace.SpanKindConsumer),
			trace.WithAttributes(config.spanAttributes...),

			// According to the specification, we should start a new root span
			// and link it to the parent span.
			trace.WithNewRoot(),
			trace.WithLinks(trace.LinkFromContext(ctxWithParentSpan)),
		)

		spanAttributes := []attribute.KeyValue{
			semconv.MessagingDestinationKindTopic,
			semconv.MessagingDestinationKey.String(message.SubscribeTopicFromCtx(ctx)),
			semconv.MessagingOperationReceive,
		}
		msgName := msg.Metadata.Get("name")
		if len(msgName) > 0 {
			spanAttributes = append(spanAttributes, semconv.MessageTypeKey.String(msgName))
		}
		span.SetAttributes(spanAttributes...)
		msg.SetContext(ctx)

		businessCtx, businessSpan := startBusinessSpan(config, msg, ctx, ctxWithParentSpan)
		msg.SetContext(businessCtx)

		events, err := h(msg)

		if err != nil {
			businessSpan.RecordError(err)
			span.RecordError(err)
		}
		businessSpan.End()
		cleanupCommandResponse(config, msg, err)
		span.End()

		return events, err
	}
}

func cleanupCommandResponse(config *config, msg *message.Message, err error) {
	if err != nil || config.commandResponseRegistry == nil {
		return
	}
	if messageKind(msg, config) != MessageKindCommandResponse {
		return
	}

	config.commandResponseRegistry.Delete(msg.Metadata.Get(wotelmeta.CorrelationID))
}

// TraceNoPublishHandler decorates a watermill NoPublishHandlerFunc to add tracing when a message is received.
func TraceNoPublishHandler(h message.NoPublishHandlerFunc, options ...Option) message.NoPublishHandlerFunc {
	decoratedHandler := TraceHandler(func(msg *message.Message) ([]*message.Message, error) {
		return nil, h(msg)
	}, options...)

	return func(msg *message.Message) error {
		_, err := decoratedHandler(msg)

		return err
	}
}

func getPropagator(config *config) propagation.TextMapPropagator {
	if config.textMapPropagator != nil {
		return config.textMapPropagator
	} else {
		return otel.GetTextMapPropagator()
	}
}
