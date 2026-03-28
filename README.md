# watermill-opentelemetry

Repository-local Watermill tracing helpers built on OpenTelemetry.

This package keeps standard Watermill transport tracing and adds a repository convention for handler process tracing over messaging.

## Concepts

The package distinguishes two tracing layers:

- transport spans: publisher and consumer spans created around broker interaction
- process spans: handler spans created for application work

Transport spans describe message movement. Process spans describe local handler execution and workflow continuation.

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

## Process tracing convention

### `event`

- consumer transport span remains a transport span
- consumer transport span starts as a new root span and links to upstream propagated context when present
- handler process span is created from the local consumer span
- handler process span links to the local transport span

### `command`

- consumer transport span remains a transport span
- first handler process span is created from the local consumer span
- when propagated upstream workflow context is present, a second handler process span is created in that workflow
- workflow-linked process span links to the local process span

### `command_response`

- reply metadata may carry callee trace context
- consumer transport span links to the propagated callee reply context
- first handler process span is created from the local consumer span
- caller workflow resumes from the registry entry saved when the command was published
- resumed workflow process span links to the local process span

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
- `command` messages register the current message context `SpanContext` in the configured command-response registry by `correlation_id` before the producer span is started

Example:

```go
publisher = wotel.NewPublisherDecorator(
    publisher,
    wotel.WithDefaultMessageKind(wotelmeta.MessageKindEvent),
)
```

## Consumer behavior

`TraceHandler(...)` still creates the Watermill consumer transport span.

It now also creates process spans for handler execution according to `message_kind`:

- `event`: consumer root span linked to upstream + one process child span linked to transport
- `command`: consumer root span + one local process child span, optionally plus a workflow-linked process span
- `command_response`: consumer root span linked to callee + one local process child span + one resumed workflow process span

The handler receives the final process context through `msg.Context()`.

## Options

- `WithTracer(...)`: transport tracer
- `WithSpanNameFunc(...)`: transport span naming
- `WithBusinessSpanNameFunc(...)`: process span naming
- `WithTextMapPropagator(...)`: custom propagator
- `WithSpanAttributes(...)`: extra transport span attributes
- `WithCommandResponseRegistry(...)`: custom reply registry
- `WithDefaultMessageKind(...)`: default message classification

## Custom attributes

This package uses a repository-specific attribute for process intent:

- `messaging.message.kind`
