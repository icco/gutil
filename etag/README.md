# etag

HTTP etag support middleware for Go.

## Installation

```
go get -u -d -v go.icco.me/gutil/etag
```

## Documentation

API documentation can be found here: https://pkg.go.dev/go.icco.me/gutil/etag

## Usage

```go
package main

import (
  "go.icco.me/gutil/etag"
  "github.com/go-chi/chi/v5"
)

func main() {
  r := chi.NewRouter()
  r.Use(etag.Handler(false))
  r.Get("/", func(w http.ResponseWriter, r *http.Request) {
    w.Write([]byte("welcome"))
  })
  http.ListenAndServe(":3000", r)
}
```
