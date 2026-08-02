package vertex

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/genai"
)

// stubResponse renders a generateContent response body with the given text and
// token counts.
func stubResponse(text string, prompt, candidates, thoughts int) string {
	body := map[string]any{
		"candidates": []any{map[string]any{
			"content": map[string]any{
				"role":  "model",
				"parts": []any{map[string]any{"text": text}},
			},
			"finishReason": "STOP",
		}},
		"usageMetadata": map[string]any{
			"promptTokenCount":     prompt,
			"candidatesTokenCount": candidates,
			"thoughtsTokenCount":   thoughts,
		},
	}
	b, _ := json.Marshal(body)
	return string(b)
}

// stub returns a client pointed at a server answering with body, plus a pointer
// to the decoded request payload the server last saw.
func stub(t *testing.T, body string) (*Client, *map[string]any) {
	t.Helper()
	seen := map[string]any{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &seen)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	c, err := New(t.Context(), Config{APIKey: "test-key", BaseURL: srv.URL, Model: "gemini-2.5-flash"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c, &seen
}

func TestNewRequiresABackend(t *testing.T) {
	t.Parallel()
	if _, err := New(t.Context(), Config{}); err == nil {
		t.Fatal("New with neither Project nor APIKey = nil, want an error")
	}
}

// Setting both would leave the backend to whichever branch happened to run
// first, and the two bill differently.
func TestNewRejectsBothBackends(t *testing.T) {
	t.Parallel()
	_, err := New(t.Context(), Config{Project: "p", APIKey: "k"})
	if err == nil {
		t.Fatal("New with both Project and APIKey = nil, want an error")
	}
	if !strings.Contains(err.Error(), "only one") {
		t.Errorf("error = %q, want it to say only one may be set", err)
	}
}

func TestNewDefaultsModel(t *testing.T) {
	t.Parallel()
	c, err := New(t.Context(), Config{APIKey: "k"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if c.Model() != DefaultModel {
		t.Errorf("Model() = %q, want %q", c.Model(), DefaultModel)
	}
}

func TestNewKeepsExplicitModel(t *testing.T) {
	t.Parallel()
	c, err := New(t.Context(), Config{APIKey: "k", Model: "gemini-2.5-pro"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if c.Model() != "gemini-2.5-pro" {
		t.Errorf("Model() = %q", c.Model())
	}
}

func TestNewVertexBackend(t *testing.T) {
	t.Parallel()
	c, err := New(t.Context(), Config{Project: "my-project"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if c.Raw() == nil {
		t.Error("Raw() = nil")
	}
}

func TestGenerateReturnsText(t *testing.T) {
	t.Parallel()
	c, _ := stub(t, stubResponse("hello there", 10, 5, 0))

	got, err := c.Generate(t.Context(), Request{Parts: Text("hi")})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got.Text != "hello there" {
		t.Errorf("Text = %q", got.Text)
	}
}

// The whole reason this package exists. genai reports thinking tokens
// separately and excludes them from CandidatesTokenCount, but the bill counts
// them as output. Reading only CandidatesTokenCount here would undercount 6x.
func TestUsageFoldsThinkingIntoOutput(t *testing.T) {
	t.Parallel()
	c, _ := stub(t, stubResponse("answer", 100, 77, 658))

	got, err := c.Generate(t.Context(), Request{Parts: Text("hi")})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got.Usage.In != 100 {
		t.Errorf("Usage.In = %d, want 100", got.Usage.In)
	}
	if got.Usage.Out != 735 {
		t.Errorf("Usage.Out = %d, want 735 (77 candidate + 658 thinking)", got.Usage.Out)
	}
}

func TestUsageWithoutMetadata(t *testing.T) {
	t.Parallel()
	body := `{"candidates":[{"content":{"role":"model","parts":[{"text":"hi"}]}}]}`
	c, _ := stub(t, body)

	got, err := c.Generate(t.Context(), Request{Parts: Text("hi")})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got.Usage.In != 0 || got.Usage.Out != 0 {
		t.Errorf("Usage = %+v, want zero when the API reported none", got.Usage)
	}
}

// A safety block or an exhausted output budget yields no text. That still cost
// money, so Usage must survive alongside the error.
func TestGenerateEmptyResponseStillReportsUsage(t *testing.T) {
	t.Parallel()
	c, _ := stub(t, stubResponse("", 42, 0, 0))

	got, err := c.Generate(t.Context(), Request{Parts: Text("hi")})
	if !errors.Is(err, ErrEmptyResponse) {
		t.Fatalf("err = %v, want ErrEmptyResponse", err)
	}
	if got == nil {
		t.Fatal("Response = nil; a budget-tracking caller needs the usage")
	}
	if got.Usage.In != 42 {
		t.Errorf("Usage.In = %d, want 42", got.Usage.In)
	}
}

func TestGenerateRejectsEmptyParts(t *testing.T) {
	t.Parallel()
	c, _ := stub(t, stubResponse("x", 1, 1, 0))

	if _, err := c.Generate(t.Context(), Request{}); err == nil {
		t.Fatal("Generate with no parts = nil, want an error")
	}
}

func TestGenerateSendsSystemInstruction(t *testing.T) {
	t.Parallel()
	c, seen := stub(t, stubResponse("ok", 1, 1, 0))

	if _, err := c.Generate(t.Context(), Request{
		System: "you are terse",
		Parts:  Text("hi"),
	}); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !strings.Contains(payload(t, seen), "you are terse") {
		t.Errorf("system instruction absent from request: %s", payload(t, seen))
	}
}

// A schema must also force the JSON response type; asking for a schema and
// getting prose back is the failure this prevents.
func TestGenerateSchemaForcesJSONMIMEType(t *testing.T) {
	t.Parallel()
	c, seen := stub(t, stubResponse(`{"a":1}`, 1, 1, 0))

	schema := &genai.Schema{
		Type:       genai.TypeObject,
		Properties: map[string]*genai.Schema{"a": {Type: genai.TypeInteger}},
	}
	if _, err := c.Generate(t.Context(), Request{Parts: Text("hi"), Schema: schema}); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	body := payload(t, seen)
	if !strings.Contains(body, "application/json") {
		t.Errorf("responseMimeType not set: %s", body)
	}
	if !strings.Contains(body, "responseSchema") {
		t.Errorf("responseSchema not sent: %s", body)
	}
}

func TestGenerateSendsTemperatureAndThinkingBudget(t *testing.T) {
	t.Parallel()
	c, seen := stub(t, stubResponse("ok", 1, 1, 0))

	if _, err := c.Generate(t.Context(), Request{
		Parts:          Text("hi"),
		Temperature:    Temperature(0.2),
		ThinkingBudget: NoThinking(),
	}); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	body := payload(t, seen)
	if !strings.Contains(body, "temperature") {
		t.Errorf("temperature not sent: %s", body)
	}
	if !strings.Contains(body, "thinkingConfig") {
		t.Errorf("thinkingConfig not sent: %s", body)
	}
}

// Omitted options must be absent, not sent as zero. A temperature of 0 is a
// meaningful setting, so it must not be indistinguishable from "unset".
func TestGenerateOmitsUnsetOptions(t *testing.T) {
	t.Parallel()
	c, seen := stub(t, stubResponse("ok", 1, 1, 0))

	if _, err := c.Generate(t.Context(), Request{Parts: Text("hi")}); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	body := payload(t, seen)
	for _, key := range []string{"temperature", "thinkingConfig", "responseSchema", "systemInstruction"} {
		if strings.Contains(body, key) {
			t.Errorf("%s sent when unset: %s", key, body)
		}
	}
}

func TestGenerateJSONDecodes(t *testing.T) {
	t.Parallel()
	c, _ := stub(t, stubResponse(`{"name":"nat","count":3}`, 10, 8, 2))

	var out struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}
	resp, err := c.GenerateJSON(t.Context(), Request{Parts: Text("hi")}, &out)
	if err != nil {
		t.Fatalf("GenerateJSON: %v", err)
	}
	if out.Name != "nat" || out.Count != 3 {
		t.Errorf("decoded %+v", out)
	}
	if resp.Usage.Out != 10 {
		t.Errorf("Usage.Out = %d, want 10", resp.Usage.Out)
	}
}

// A decode failure still cost tokens, so the caller must get the usage back.
func TestGenerateJSONReturnsUsageOnDecodeFailure(t *testing.T) {
	t.Parallel()
	c, _ := stub(t, stubResponse("not json at all", 10, 5, 0))

	var out map[string]any
	resp, err := c.GenerateJSON(t.Context(), Request{Parts: Text("hi")}, &out)
	if err == nil {
		t.Fatal("GenerateJSON on prose = nil, want a decode error")
	}
	if resp == nil || resp.Usage.In != 10 {
		t.Errorf("usage lost on decode failure: %+v", resp)
	}
}

func TestGenerateJSONPropagatesEmptyResponse(t *testing.T) {
	t.Parallel()
	c, _ := stub(t, stubResponse("", 5, 0, 0))

	var out map[string]any
	if _, err := c.GenerateJSON(t.Context(), Request{Parts: Text("hi")}, &out); !errors.Is(err, ErrEmptyResponse) {
		t.Fatalf("err = %v, want ErrEmptyResponse", err)
	}
}

func TestGenerateModelOverride(t *testing.T) {
	t.Parallel()
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_, _ = w.Write([]byte(stubResponse("ok", 1, 1, 0)))
	}))
	defer srv.Close()

	c, err := New(t.Context(), Config{APIKey: "k", BaseURL: srv.URL, Model: "gemini-2.5-flash"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := c.Generate(t.Context(), Request{Parts: Text("hi"), Model: "gemini-2.5-pro"}); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !strings.Contains(path, "gemini-2.5-pro") {
		t.Errorf("request path = %q, want the per-request model override", path)
	}
}

func TestGenerateSurfacesAPIError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"code":429,"message":"quota"}}`))
	}))
	defer srv.Close()

	c, err := New(t.Context(), Config{APIKey: "k", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := c.Generate(t.Context(), Request{Parts: Text("hi")}); err == nil {
		t.Fatal("Generate on a 429 = nil, want an error")
	}
}

func TestTextHelper(t *testing.T) {
	t.Parallel()
	parts := Text("hello")
	if len(parts) != 1 || parts[0].Text != "hello" {
		t.Errorf("Text() = %+v", parts)
	}
}

func TestBlobHelper(t *testing.T) {
	t.Parallel()
	p := Blob("image/png", []byte{1, 2, 3})
	if p.InlineData == nil {
		t.Fatal("Blob() produced no inline data")
	}
	if p.InlineData.MIMEType != "image/png" || len(p.InlineData.Data) != 3 {
		t.Errorf("Blob() = %+v", p.InlineData)
	}
}

// The prompt must precede the media it applies to.
func TestTextWithBlobOrdersPromptFirst(t *testing.T) {
	t.Parallel()
	parts := TextWithBlob("describe this", "image/jpeg", []byte{9})
	if len(parts) != 2 {
		t.Fatalf("got %d parts, want 2", len(parts))
	}
	if parts[0].Text != "describe this" {
		t.Errorf("first part = %+v, want the prompt", parts[0])
	}
	if parts[1].InlineData == nil {
		t.Errorf("second part = %+v, want the blob", parts[1])
	}
}

func TestNoThinkingIsZero(t *testing.T) {
	t.Parallel()
	if got := NoThinking(); got == nil || *got != 0 {
		t.Errorf("NoThinking() = %v, want a pointer to 0", got)
	}
}

func TestTemperatureHelper(t *testing.T) {
	t.Parallel()
	if got := Temperature(0.7); got == nil || *got != 0.7 {
		t.Errorf("Temperature(0.7) = %v", got)
	}
}

// payload renders the last request body the stub saw.
func payload(t *testing.T, seen *map[string]any) string {
	t.Helper()
	b, err := json.Marshal(*seen)
	if err != nil {
		t.Fatalf("marshal seen request: %v", err)
	}
	return string(b)
}
