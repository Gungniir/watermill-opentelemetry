package opentelemetry

import (
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// config represents the configuration options available for subscriber
// middlewares and publisher decorators.
type config struct {
	spanAttributes          []attribute.KeyValue
	textMapPropagator       propagation.TextMapPropagator
	tracer                  trace.Tracer
	consumeSpanNameFunc     func(subscriberOrPublisherName, topic string) string
	processSpanNameFunc     func(handler, topic, messageKind string) string
	commandResponseRegistry CommandResponseRegistry
	defaultMessageKind      string
}

// Option provides a convenience wrapper for simple options that can be
// represented as functions.
type Option func(*config)

// WithSpanAttributes includes the given attributes to the generated Spans.
func WithSpanAttributes(attributes ...attribute.KeyValue) Option {
	return func(c *config) {
		c.spanAttributes = attributes
	}
}

// WithTextMapPropagator sets propagator.
func WithTextMapPropagator(p propagation.TextMapPropagator) Option {
	return func(c *config) {
		c.textMapPropagator = p
	}
}

// WithTracer sets tracer.
func WithTracer(t trace.Tracer) Option {
	return func(c *config) {
		c.tracer = t
	}
}

// WithConsumeSpanNameFunc sets SpanNameFunc.
func WithConsumeSpanNameFunc(spanNameFunc func(subscriberOrPublisherName, topic string) string) Option {
	return func(c *config) {
		c.consumeSpanNameFunc = spanNameFunc
	}
}

// WithProcessSpanNameFunc sets naming for business spans created for handlers.
func WithProcessSpanNameFunc(spanNameFunc func(handler, topic, messageKind string) string) Option {
	return func(c *config) {
		c.processSpanNameFunc = spanNameFunc
	}
}

// WithCommandResponseRegistry sets command response registry implementation.
func WithCommandResponseRegistry(registry CommandResponseRegistry) Option {
	return func(c *config) {
		c.commandResponseRegistry = registry
	}
}

// WithDefaultMessageKind sets a default message kind for messages that omit metadata.
func WithDefaultMessageKind(kind string) Option {
	return func(c *config) {
		c.defaultMessageKind = kind
	}
}
