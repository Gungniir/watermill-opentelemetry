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
		consumeSpanNameFunc: func(handler, topic string) string {
			return fmt.Sprintf("%s: %s", handler, topic)
		},
		processSpanNameFunc: func(handler, topic, kind string) string {
			return fmt.Sprintf("%s: %s [%s]", handler, topic, kind)
		},
		commandResponseRegistry: defaultCommandResponseRegistry,
	}

	for _, opt := range options {
		opt(config)
	}

	return func(msg *message.Message) ([]*message.Message, error) {
		msgctx := msg.Context()
		ctxWithParentSpan := getPropagator(config).Extract(msgctx, metadataWrapper{msg.Metadata})

		handlerName := message.HandlerNameFromCtx(ctxWithParentSpan)
		topicName := message.SubscribeTopicFromCtx(msgctx)

		consumeSpanCtx, consumeSpan := config.tracer.Start(ctxWithParentSpan, config.consumeSpanNameFunc(handlerName, topicName),
			trace.WithSpanKind(trace.SpanKindConsumer),
			trace.WithAttributes(config.spanAttributes...),

			// According to the specification, we should start a new root span
			// and link it to the parent span.
			trace.WithNewRoot(),
			trace.WithLinks(trace.LinkFromContext(ctxWithParentSpan)),
		)

		spanAttributes := []attribute.KeyValue{
			semconv.MessagingDestinationKindTopic,
			semconv.MessagingDestinationKey.String(message.SubscribeTopicFromCtx(consumeSpanCtx)),
			semconv.MessagingOperationReceive,
		}
		msgName := msg.Metadata.Get("name")
		if len(msgName) > 0 {
			spanAttributes = append(spanAttributes, semconv.MessageTypeKey.String(msgName))
		}
		consumeSpan.SetAttributes(spanAttributes...)

		ctx, processSpan, linkedProcessSpan := startProcessSpan(config, msg, consumeSpanCtx, ctxWithParentSpan)
		msg.SetContext(ctx)

		events, err := h(msg)

		if err != nil {
			processSpan.RecordError(err)
			if linkedProcessSpan != nil {
				linkedProcessSpan.RecordError(err)
			}
			consumeSpan.RecordError(err)
		}

		processSpan.End()
		if linkedProcessSpan != nil {
			linkedProcessSpan.End()
		}

		if messageKind(msg, config) == MessageKindCommandResponse {
			config.commandResponseRegistry.Delete(msg.Metadata.Get(wotelmeta.CorrelationID))
		}
		consumeSpan.End()

		return events, err
	}
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
	}

	return otel.GetTextMapPropagator()
}
