# OpenTelemetry example

Tiny HTTP server that wires `gutil/logging.Middleware` together with
[`otelhttp`](https://pkg.go.dev/go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp)
so request logs and traces share the same `trace_id` / `span_id`.

This is a separate Go module so the OTel SDK and contrib dependencies
don't bleed into the main `gutil` module. A `replace` directive points
at the in-tree `gutil`, so `go run .` works directly from this checkout.

## Run it

```sh
cd examples/otel
go run .
# in another terminal
curl -i http://localhost:8080/hello
```

You'll see two things in the server's stdout:

1. A structured request log from `logging.Middleware` containing
   `trace_id`, `span_id`, and `trace_sampled` fields.
2. A pretty-printed `http.server` span from the stdout exporter whose
   `TraceID` / `SpanID` match the log line above.

## How it fits together

```
incoming request
      |
      v
otelhttp.NewHandler(r, "http.server")   <-- creates server span,
      |                                     puts it on r.Context()
      v
chi.Router with logging.Middleware      <-- reads the span context,
      |                                     stamps trace_id/span_id
      v                                     onto request log
your handlers
```

The key pieces of `gutil` you're exercising:

- `logging.NewLogger("service-name")` — plain zap production logger
  tagged with `service`.
- `logging.Middleware(logger.Desugar())` — chi middleware that emits
  one structured log per request and, when an OpenTelemetry span is
  present on the request context, includes `trace_id`, `span_id`, and
  `trace_sampled`.

The OTel setup itself (tracer provider, exporter, propagator) lives in
this example, not in `gutil` — pick whatever exporter and sampler your
deployment needs.
