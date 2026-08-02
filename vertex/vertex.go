// Package vertex is a thin wrapper over google.golang.org/genai for the way
// these services actually use Gemini: one request, a JSON-constrained response,
// and an honest token count.
//
// It exists because five repos had each hand-rolled the same twenty lines, and
// one of them had learned something the others hadn't. See [Usage] for what
// that was.
//
// Both backends are supported: Vertex AI (project + location, authenticated by
// Application Default Credentials) and the Gemini API (an API key).
package vertex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"google.golang.org/genai"
)

// DefaultLocation is the Vertex region used when Config.Location is empty.
const DefaultLocation = "us-central1"

// DefaultModel is used when neither Config.Model nor Request.Model is set.
const DefaultModel = "gemini-2.5-flash"

// ErrEmptyResponse is returned when the model produces no text. It is a normal
// outcome — a safety block or an exhausted token budget both land here — so it
// is a sentinel rather than a generic error.
var ErrEmptyResponse = errors.New("vertex: model returned no text")

// Config selects a backend and a default model.
//
// Set Project for Vertex AI, or APIKey for the Gemini API. Setting both is an
// error rather than a silent preference, since the two bill differently.
type Config struct {
	// Project is the GCP project for the Vertex AI backend. Auth is by
	// Application Default Credentials.
	Project string
	// Location is the Vertex region. Empty means DefaultLocation.
	Location string
	// APIKey selects the Gemini API backend instead of Vertex AI.
	APIKey string
	// Model is the default model id. Empty means DefaultModel.
	Model string
	// BaseURL overrides the API endpoint, mostly so tests can point at a stub.
	BaseURL string
	// HTTPClient overrides the HTTP client, for a caller that wants its own
	// timeout, transport, or instrumentation.
	//
	// On the Vertex backend this also takes over authentication: genai skips
	// Application Default Credentials entirely when a client is supplied, so the
	// client must carry its own.
	HTTPClient *http.Client
}

// Client talks to one Gemini backend.
type Client struct {
	genai *genai.Client
	model string
}

// New builds a client. It returns an error if neither Project nor APIKey is
// set, or if both are.
func New(ctx context.Context, cfg Config) (*Client, error) {
	switch {
	case cfg.Project == "" && cfg.APIKey == "":
		return nil, errors.New("vertex: set Config.Project (Vertex AI) or Config.APIKey (Gemini API)")
	case cfg.Project != "" && cfg.APIKey != "":
		return nil, errors.New("vertex: set only one of Config.Project and Config.APIKey")
	}

	gc := &genai.ClientConfig{}
	if cfg.APIKey != "" {
		gc.Backend = genai.BackendGeminiAPI
		gc.APIKey = cfg.APIKey
	} else {
		gc.Backend = genai.BackendVertexAI
		gc.Project = cfg.Project
		gc.Location = cfg.Location
		if gc.Location == "" {
			gc.Location = DefaultLocation
		}
	}

	if cfg.BaseURL != "" {
		gc.HTTPOptions.BaseURL = cfg.BaseURL
	}
	if cfg.HTTPClient != nil {
		gc.HTTPClient = cfg.HTTPClient
	}

	c, err := genai.NewClient(ctx, gc)
	if err != nil {
		return nil, fmt.Errorf("vertex: new genai client: %w", err)
	}

	model := cfg.Model
	if model == "" {
		model = DefaultModel
	}
	return &Client{genai: c, model: model}, nil
}

// Model reports the client's default model id.
func (c *Client) Model() string { return c.model }

// Raw exposes the underlying genai client for anything this wrapper does not
// cover — streaming, embeddings, the file API.
func (c *Client) Raw() *genai.Client { return c.genai }

// Request is one generation call.
type Request struct {
	// System is an optional system instruction.
	System string
	// Parts is the user turn. Use Text, Blob, or TextWithBlob to build it.
	Parts []*genai.Part
	// Schema constrains the response to JSON matching it. When set, the
	// response MIME type is forced to application/json.
	Schema *genai.Schema
	// Temperature overrides the model default when non-nil.
	Temperature *float32
	// ThinkingBudget caps reasoning tokens when non-nil. Set it to zero for
	// mechanical tasks: thinking bills as output whether or not it helped.
	ThinkingBudget *int32
	// Model overrides the client default for this call.
	Model string
}

// Usage is the token cost of one call.
type Usage struct {
	// In is the prompt token count.
	In int
	// Out is the billable output: candidate tokens *plus thinking tokens*.
	//
	// This is the part worth centralizing. genai reports thinking separately in
	// ThoughtsTokenCount and does not include it in CandidatesTokenCount, but
	// the bill counts it as output. Reading only CandidatesTokenCount undercounts
	// badly — measured on gemini-2.5-pro, 77 candidate tokens against 658
	// thinking tokens, a 9.5x miss. One service ran up $737 in a month with its
	// own accounting reporting nearly zero.
	Out int
}

// Response is the model's answer plus what it cost.
type Response struct {
	Text  string
	Usage Usage
}

// Generate performs one call and returns the response text.
//
// The returned Response is never nil, so a caller enforcing a budget can read
// Usage unconditionally. It is populated whenever the API reported it, which
// includes the ErrEmptyResponse case — an empty answer still costs money — and
// is zero when the call never reached the model.
//
// It returns ErrEmptyResponse when the model produces no text, which happens on
// a safety block or an exhausted output budget.
func (c *Client) Generate(ctx context.Context, r Request) (*Response, error) {
	if len(r.Parts) == 0 {
		return &Response{}, errors.New("vertex: Request.Parts is empty")
	}

	cfg := &genai.GenerateContentConfig{}
	if r.System != "" {
		cfg.SystemInstruction = &genai.Content{Parts: []*genai.Part{{Text: r.System}}}
	}
	if r.Schema != nil {
		cfg.ResponseMIMEType = "application/json"
		cfg.ResponseSchema = r.Schema
	}
	if r.Temperature != nil {
		cfg.Temperature = r.Temperature
	}
	if r.ThinkingBudget != nil {
		cfg.ThinkingConfig = &genai.ThinkingConfig{ThinkingBudget: r.ThinkingBudget}
	}

	model := r.Model
	if model == "" {
		model = c.model
	}

	resp, err := c.genai.Models.GenerateContent(ctx, model, []*genai.Content{{Parts: r.Parts}}, cfg)
	if err != nil {
		// Non-nil with zero Usage: the call never reached the model, so there is
		// nothing to bill, but the caller should not have to nil-check.
		return &Response{}, fmt.Errorf("vertex: generate: %w", err)
	}

	out := &Response{Usage: usageOf(resp)}
	out.Text = strings.TrimSpace(resp.Text())
	if out.Text == "" {
		return out, ErrEmptyResponse
	}
	return out, nil
}

// GenerateJSON calls Generate and unmarshals the response into v. Set
// Request.Schema so the model is constrained to produce parseable output;
// without it the model is free to wrap the JSON in prose and this will fail.
//
// As with Generate, the returned Response is never nil, so its Usage still
// counts against a budget even on a decode failure.
func (c *Client) GenerateJSON(ctx context.Context, r Request, v any) (*Response, error) {
	resp, err := c.Generate(ctx, r)
	if err != nil {
		return resp, err
	}
	if err := json.Unmarshal([]byte(resp.Text), v); err != nil {
		return resp, fmt.Errorf("vertex: decode response: %w", err)
	}
	return resp, nil
}

// usageOf reads the token counts off a response, folding thinking tokens into
// the output total. See Usage.Out.
func usageOf(resp *genai.GenerateContentResponse) Usage {
	if resp == nil || resp.UsageMetadata == nil {
		return Usage{}
	}
	u := resp.UsageMetadata
	return Usage{
		In:  int(u.PromptTokenCount),
		Out: int(u.CandidatesTokenCount) + int(u.ThoughtsTokenCount),
	}
}

// Text returns a Parts slice holding a single text prompt.
func Text(prompt string) []*genai.Part {
	return []*genai.Part{{Text: prompt}}
}

// Blob returns an inline-data part, for sending an image, audio clip, or PDF.
func Blob(mimeType string, data []byte) *genai.Part {
	return &genai.Part{InlineData: &genai.Blob{Data: data, MIMEType: mimeType}}
}

// TextWithBlob returns a Parts slice holding a prompt followed by inline data.
// The prompt goes first: Gemini attends better to an instruction that precedes
// the media it applies to.
func TextWithBlob(prompt, mimeType string, data []byte) []*genai.Part {
	return []*genai.Part{{Text: prompt}, Blob(mimeType, data)}
}

// NoThinking is the ThinkingBudget for tasks that need no reasoning. Thinking
// bills as output whether or not it improved the answer, so classification and
// extraction should usually set it.
func NoThinking() *int32 { return genai.Ptr[int32](0) }

// Temperature returns a pointer to t, for Request.Temperature.
func Temperature(t float32) *float32 { return &t }
