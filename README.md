# watermill-opentelemetry

Repository-local Watermill tracing helpers built on OpenTelemetry.

This package keeps standard Watermill transport tracing and adds a repository convention for business tracing over messaging.

## Concepts

The package distinguishes two tracing layers:

- transport spans: publisher and consumer spans created around broker interaction
- business spans: handler spans created for application work

Transport spans describe message movement. Business spans describe workflow causality.

## Message kinds

Message classification is explicit and carried in message metadata:

- `event`
- `command`
- `command_response`

Metadata keys and helpers live in:

- `github.com/Gungniir/watermill-opentelemetry/pkg/opentelemetry/metadata`

Typical import alias:

```go
import wotelmeta "github.com/Gungniir/watermill-opentelemetry/pkg/opentelemetry/metadata"
```

## Business tracing convention

### `event`

- consumer transport span remains a transport span
- handler business span starts as a new root span
- business span links to upstream propagated context when present
- business span links to local transport span when present

### `command`

- consumer transport span remains a transport span
- handler business span continues the propagated workflow context
- business span is not a child of the local transport span
- business span links to the local transport span

### `command_response`

- reply metadata may carry callee trace context
- caller business work must resume the original workflow context saved when the command was sent
- business span links to callee reply context
- business span links to local transport span

## Registry

`command_response` handling needs access to the original caller workflow context.

The package provides:

- `CommandResponseRegistry` interface
- `NewInMemoryCommandResponseRegistry()` default in-memory implementation

Custom implementations can be injected with:

```go
wotel.WithCommandResponseRegistry(registry)
```

## Publisher behavior

`NewPublisherDecorator(...)` still creates transport publish spans and injects propagated trace context into message metadata.

Additional behavior:

- `WithDefaultMessageKind(kind)` sets `message_kind` when the message does not already declare one
- `command` messages register the current workflow `SpanContext` in the configured command-response registry by `correlation_id`

Example:

```go
publisher = wotel.NewPublisherDecorator(
    publisher,
    wotel.WithDefaultMessageKind(wotelmeta.MessageKindEvent),
)
```

## Consumer behavior

`TraceHandler(...)` still creates the Watermill consumer transport span.

It now also creates a business span for handler execution according to `message_kind`:

- `event`: new root + links
- `command`: continue workflow context + link transport
- `command_response`: resume original workflow context from registry + links

The handler receives the business-shaped context through `msg.Context()`.

## Options

- `WithTracer(...)`: transport tracer
- `WithSpanNameFunc(...)`: transport span naming
- `WithBusinessTracer(...)`: business tracer
- `WithBusinessSpanNameFunc(...)`: business span naming
- `WithTextMapPropagator(...)`: custom propagator
- `WithSpanAttributes(...)`: extra transport span attributes
- `WithCommandResponseRegistry(...)`: custom reply registry
- `WithDefaultMessageKind(...)`: default message classification

## Custom attributes

This package uses a repository-specific attribute for business intent:

- `messaging.message.kind`

Related diagnostic attributes:

- `messaging.message.context_missing`
- `messaging.message.command_response_registry_miss`
- `messaging.message.invalid_kind`
