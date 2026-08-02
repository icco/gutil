# vertex

A thin wrapper over [`google.golang.org/genai`](https://pkg.go.dev/google.golang.org/genai) for the way these services actually use Gemini: one request, a JSON-constrained response, and an honest token count.

It exists because five repos had each hand-rolled the same twenty lines, and one of them had learned something the others hadn't — see [Token accounting](#token-accounting).

## Installation

```
go get -u -d -v github.com/icco/gutil/vertex
```

## Documentation

API documentation can be found here: https://pkg.go.dev/github.com/icco/gutil/vertex

## Usage

```go
c, err := vertex.New(ctx, vertex.Config{
  Project:  os.Getenv("GOOGLE_CLOUD_PROJECT"), // Vertex AI, auth by ADC
  Location: "us-central1",
  Model:    "gemini-2.5-flash",
})
if err != nil {
  return err
}

var out struct {
  Category   string  `json:"category"`
  Confidence float64 `json:"confidence"`
}

resp, err := c.GenerateJSON(ctx, vertex.Request{
  System: "Sort each message into exactly one category.",
  Parts:  vertex.Text(body),
  Schema: &genai.Schema{
    Type: genai.TypeObject,
    Properties: map[string]*genai.Schema{
      "category":   {Type: genai.TypeString, Enum: []string{"keep", "archive", "trash"}},
      "confidence": {Type: genai.TypeNumber},
    },
  },
  // Sorting needs no reasoning, and reasoning bills as output.
  ThinkingBudget: vertex.NoThinking(),
}, &out)
if errors.Is(err, vertex.ErrEmptyResponse) {
  // Safety block or exhausted output budget. resp.Usage is still populated.
} else if err != nil {
  return err
}

spend.Record(c.Model(), resp.Usage.In, resp.Usage.Out)
```

### The Gemini API backend

Set `APIKey` instead of `Project`. Setting both is an error rather than a silent preference, because the two bill differently.

```go
c, err := vertex.New(ctx, vertex.Config{APIKey: os.Getenv("GEMINI_API_KEY")})
```

### Images, audio, PDFs

```go
resp, err := c.Generate(ctx, vertex.Request{
  Parts: vertex.TextWithBlob("Describe this image.", "image/png", data),
})
```

## Token accounting

**`Usage.Out` is candidate tokens plus thinking tokens.** This is the part worth centralizing.

genai reports thinking separately in `ThoughtsTokenCount` and does **not** include it in `CandidatesTokenCount` — but the bill counts it as output. Reading only `CandidatesTokenCount`, which is the obvious thing to do, undercounts badly: measured on `gemini-2.5-pro`, 77 candidate tokens against 658 thinking tokens, a 9.5x miss. One service ran up $737 in a month while its own accounting reported nearly zero.

Two consequences the API is shaped around:

- The returned `*Response` is never nil, so a caller tracking spend reads `Usage` unconditionally. It is populated whenever the API reported it — including the `ErrEmptyResponse` and decode-failure paths, which still cost money — and zero when the call never reached the model.
- `ThinkingBudget` is easy to reach for. `vertex.NoThinking()` sets it to zero, which is usually right for classification and extraction — thinking bills whether or not it improved the answer.

## Notes

- **A schema forces the JSON response type.** Setting `Request.Schema` sets `ResponseMIMEType` to `application/json` as well, so the model can't wrap its answer in prose. `GenerateJSON` without a schema will work sometimes and fail confusingly the rest of the time.
- **Unset options are omitted, not zeroed.** A temperature of 0 is a meaningful setting, so `Temperature` and `ThinkingBudget` are pointers; leaving them nil sends nothing.
- **`ErrEmptyResponse` is a normal outcome**, not a transport failure — a safety block or an exhausted output budget both land there. Match it with `errors.Is`.
- **`Raw()` is the escape hatch** for anything this doesn't cover: streaming, embeddings, the file API.
- `BaseURL` exists so tests can point at a stub server.
