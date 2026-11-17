# gutil

[![Go Reference](https://pkg.go.dev/badge/go.icco.me/gutil.svg)](https://pkg.go.dev/go.icco.me/gutil) 
[![golangci-lint](https://github.com/icco/gutil/actions/workflows/golangci-lint.yml/badge.svg)](https://github.com/icco/gutil/actions/workflows/golangci-lint.yml)

Common golang libraries for my projects.

## Tracing

```go
shutdown, err := tracing.Init(ctx, "hello")
if err != nil {
	return err
}
defer shutdown(context.Background())
```

[`tracing.Init`](https://pkg.go.dev/go.icco.me/gutil/tracing) exports spans over OTLP/HTTP when `OTEL_EXPORTER_OTLP_ENDPOINT` is set. Wrap handlers with `otelhttp` so `logging.Middleware` stamps `trace_id` on request logs.
