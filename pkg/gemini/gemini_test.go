package gemini_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"todo-app/pkg/gemini"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	geAPIKey = "AIzaSyTESTKEY-do-not-log-0123456789"
	geModel  = "gemini-test-model"
)

// geCapture is one request as the fake provider saw it.
type geCapture struct {
	path string
	key  string
	body []byte
}

// geRecorder collects requests under a mutex so assertions never race with the
// httptest handler goroutine.
type geRecorder struct {
	mu   sync.Mutex
	reqs []geCapture
}

func (r *geRecorder) add(req *http.Request) {
	body, _ := io.ReadAll(req.Body)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reqs = append(r.reqs, geCapture{path: req.URL.Path, key: req.URL.Query().Get("key"), body: body})
}

func (r *geRecorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.reqs)
}

func (r *geRecorder) last() geCapture {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.reqs[len(r.reqs)-1]
}

// geServer spins up a fake provider and returns a client already pointed at it.
func geServer(t *testing.T, cfg gemini.Config, handler http.HandlerFunc) (*gemini.HTTPClient, *geRecorder) {
	t.Helper()

	rec := &geRecorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.add(r)
		handler(w, r)
	}))
	t.Cleanup(srv.Close)

	if cfg.APIKey == "" {
		cfg.APIKey = geAPIKey
	}
	if cfg.Model == "" {
		cfg.Model = geModel
	}
	cfg.BaseURL = srv.URL
	if cfg.Timeout == 0 {
		cfg.Timeout = 5 * time.Second
	}

	return gemini.New(cfg), rec
}

// geCandidates renders a well-formed 2xx envelope whose single candidate is
// split across the given parts.
func geCandidates(parts ...string) string {
	partObjs := make([]any, 0, len(parts))
	for _, p := range parts {
		partObjs = append(partObjs, map[string]any{"text": p})
	}
	body, err := json.Marshal(map[string]any{
		"candidates": []any{
			map[string]any{
				"content":      map[string]any{"parts": partObjs, "role": "model"},
				"finishReason": "STOP",
			},
		},
	})
	if err != nil {
		panic(err)
	}
	return string(body)
}

func geJSONHandler(status int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}
}

func geFloat(v float64) *float64 { return &v }

// geDecodeBody turns a captured request body into a generic map.
func geDecodeBody(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var out map[string]any
	require.NoError(t, json.Unmarshal(raw, &out), "request body must be valid JSON")
	return out
}

// geRoundTrip normalises a Go value through JSON so it can be compared against
// something the server decoded out of the wire format.
func geRoundTrip(t *testing.T, v any) any {
	t.Helper()
	raw, err := json.Marshal(v)
	require.NoError(t, err)
	var out any
	require.NoError(t, json.Unmarshal(raw, &out))
	return out
}

func TestHTTPClient_GenerateJSON_RequestShape(t *testing.T) {
	schema := gemini.Object(map[string]any{
		"headline": gemini.Str("one line"),
		"score":    gemini.Num("0..100"),
		"trend":    gemini.Enum("direction", "UP", "FLAT", "DOWN"),
		"tags":     gemini.Array(gemini.Str("")),
	}, "headline", "score")

	t.Run("happy path returns parsed JSON and sends the documented payload", func(t *testing.T) {
		client, rec := geServer(t, gemini.Config{Temperature: 0.7},
			geJSONHandler(http.StatusOK, geCandidates(`{"headline":"all good","score":91}`)))

		out, err := client.GenerateJSON(context.Background(), gemini.Request{
			System:          "you are a planner",
			Prompt:          "summarise my week",
			Schema:          schema,
			MaxOutputTokens: 2048,
		})

		require.NoError(t, err)
		assert.JSONEq(t, `{"headline":"all good","score":91}`, string(out))
		require.Equal(t, 1, rec.count())

		got := rec.last()
		assert.Equal(t, "/models/"+geModel+":generateContent", got.path)
		assert.Equal(t, geAPIKey, got.key, "api key travels in the key query parameter")

		body := geDecodeBody(t, got.body)

		system, ok := body["systemInstruction"].(map[string]any)
		require.True(t, ok, "systemInstruction must be present when System is set")
		sysParts, ok := system["parts"].([]any)
		require.True(t, ok)
		require.Len(t, sysParts, 1)
		assert.Equal(t, "you are a planner", sysParts[0].(map[string]any)["text"])

		contents, ok := body["contents"].([]any)
		require.True(t, ok, "contents must be present")
		require.Len(t, contents, 1)
		userTurn := contents[0].(map[string]any)
		assert.Equal(t, "user", userTurn["role"])
		userParts, ok := userTurn["parts"].([]any)
		require.True(t, ok)
		require.Len(t, userParts, 1)
		assert.Equal(t, "summarise my week", userParts[0].(map[string]any)["text"])

		genCfg, ok := body["generationConfig"].(map[string]any)
		require.True(t, ok, "generationConfig must be present")
		assert.Equal(t, "application/json", genCfg["responseMimeType"])
		assert.Equal(t, geRoundTrip(t, schema), genCfg["responseSchema"], "responseSchema must be forwarded verbatim")
		assert.InDelta(t, 0.7, genCfg["temperature"], 1e-9)
		assert.EqualValues(t, 2048, genCfg["maxOutputTokens"])
	})

	t.Run("systemInstruction is absent when System is empty", func(t *testing.T) {
		cases := []struct {
			name   string
			system string
		}{
			{name: "empty string", system: ""},
			{name: "whitespace only", system: "   \n\t "},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				client, rec := geServer(t, gemini.Config{},
					geJSONHandler(http.StatusOK, geCandidates(`{"ok":true}`)))

				_, err := client.GenerateJSON(context.Background(), gemini.Request{
					System: tc.system,
					Prompt: "go",
					Schema: schema,
				})

				require.NoError(t, err)
				require.Equal(t, 1, rec.count())

				body := geDecodeBody(t, rec.last().body)
				assert.NotContains(t, body, "systemInstruction")
			})
		}
	})

	t.Run("request temperature overrides the client default", func(t *testing.T) {
		client, rec := geServer(t, gemini.Config{Temperature: 0.9},
			geJSONHandler(http.StatusOK, geCandidates(`{"ok":true}`)))

		_, err := client.GenerateJSON(context.Background(), gemini.Request{
			Prompt:      "go",
			Schema:      schema,
			Temperature: geFloat(0),
		})

		require.NoError(t, err)
		genCfg := geDecodeBody(t, rec.last().body)["generationConfig"].(map[string]any)
		assert.InDelta(t, 0.0, genCfg["temperature"], 1e-9)
	})

	t.Run("Model reports the configured model", func(t *testing.T) {
		client, _ := geServer(t, gemini.Config{Model: "gemini-custom"},
			geJSONHandler(http.StatusOK, geCandidates(`{"ok":true}`)))
		assert.Equal(t, "gemini-custom", client.Model())
	})
}

func TestHTTPClient_GenerateJSON_TextUnwrapping(t *testing.T) {
	cases := []struct {
		name  string
		parts []string
		want  string
	}{
		{
			name:  "plain json",
			parts: []string{`{"ok":true,"n":1}`},
			want:  `{"ok":true,"n":1}`,
		},
		{
			name:  "triple backtick json fence",
			parts: []string{"```json\n{\"ok\":true,\"n\":1}\n```"},
			want:  `{"ok":true,"n":1}`,
		},
		{
			name:  "bare triple backtick fence",
			parts: []string{"```\n{\"ok\":true,\"n\":1}\n```"},
			want:  `{"ok":true,"n":1}`,
		},
		{
			name:  "fence with surrounding whitespace",
			parts: []string{"\n\n  ```json\n{\"ok\":true,\"n\":1}\n```  \n"},
			want:  `{"ok":true,"n":1}`,
		},
		{
			name:  "fenced array",
			parts: []string{"```json\n[1,2,3]\n```"},
			want:  `[1,2,3]`,
		},
		{
			name:  "text split across parts",
			parts: []string{"```json\n{\"ok\":", "true,\"n\":1}\n```"},
			want:  `{"ok":true,"n":1}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client, rec := geServer(t, gemini.Config{},
				geJSONHandler(http.StatusOK, geCandidates(tc.parts...)))

			out, err := client.GenerateJSON(context.Background(), gemini.Request{Prompt: "go"})

			require.NoError(t, err)
			assert.True(t, json.Valid(out), "returned payload must be valid JSON, got %q", string(out))
			assert.JSONEq(t, tc.want, string(out))
			assert.Equal(t, 1, rec.count())
		})
	}
}

func TestHTTPClient_GenerateJSON_Retries(t *testing.T) {
	t.Run("5xx is retried up to MaxRetries and returns ErrTransient", func(t *testing.T) {
		cases := []struct {
			name         string
			maxRetries   int
			status       int
			wantRequests int
		}{
			{name: "one retry on 500", maxRetries: 1, status: http.StatusInternalServerError, wantRequests: 2},
			{name: "two retries on 500", maxRetries: 2, status: http.StatusInternalServerError, wantRequests: 3},
			{name: "one retry on 503", maxRetries: 1, status: http.StatusServiceUnavailable, wantRequests: 2},
			{name: "one retry on 429", maxRetries: 1, status: http.StatusTooManyRequests, wantRequests: 2},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				client, rec := geServer(t, gemini.Config{MaxRetries: tc.maxRetries},
					geJSONHandler(tc.status, `{"error":{"message":"upstream exploded"}}`))

				out, err := client.GenerateJSON(context.Background(), gemini.Request{Prompt: "go"})

				require.Error(t, err)
				assert.Nil(t, out)
				assert.ErrorIs(t, err, gemini.ErrTransient)
				assert.NotErrorIs(t, err, gemini.ErrBadResponse)
				assert.Equal(t, tc.wantRequests, rec.count(),
					"MaxRetries=%d must produce exactly %d requests", tc.maxRetries, tc.wantRequests)
			})
		}
	})

	t.Run("4xx is not retried", func(t *testing.T) {
		cases := []struct {
			name   string
			status int
		}{
			{name: "400 bad request", status: http.StatusBadRequest},
			{name: "401 unauthorized", status: http.StatusUnauthorized},
			{name: "403 forbidden", status: http.StatusForbidden},
			{name: "404 not found", status: http.StatusNotFound},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				client, rec := geServer(t, gemini.Config{MaxRetries: 3},
					geJSONHandler(tc.status, `{"error":{"message":"nope"}}`))

				out, err := client.GenerateJSON(context.Background(), gemini.Request{Prompt: "go"})

				require.Error(t, err)
				assert.Nil(t, out)
				assert.NotErrorIs(t, err, gemini.ErrTransient, "client errors must not be classified as transient")
				assert.Equal(t, 1, rec.count(), "a %d must be attempted exactly once", tc.status)
				assert.Contains(t, err.Error(), fmt.Sprintf("status %d", tc.status))
			})
		}
	})
}

func TestHTTPClient_GenerateJSON_BadResponses(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{name: "no candidates key", body: `{}`},
		{name: "empty candidates array", body: `{"candidates":[]}`},
		{name: "candidate with no parts", body: `{"candidates":[{"content":{"parts":[]},"finishReason":"SAFETY"}]}`},
		{name: "empty text", body: geCandidates("")},
		{name: "whitespace only text", body: geCandidates("   \n\t  ")},
		{name: "text is not valid JSON", body: geCandidates("I am sorry, I cannot comply.")},
		{name: "text is a truncated object", body: geCandidates(`{"headline":"cut off`)},
		{name: "fenced text is not valid JSON", body: geCandidates("```json\nnot json at all\n```")},
		{name: "envelope is not JSON", body: `<html>502 Bad Gateway</html>`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client, rec := geServer(t, gemini.Config{MaxRetries: 3},
				geJSONHandler(http.StatusOK, tc.body))

			out, err := client.GenerateJSON(context.Background(), gemini.Request{Prompt: "go"})

			require.Error(t, err)
			assert.Nil(t, out)
			assert.ErrorIs(t, err, gemini.ErrBadResponse)
			assert.NotErrorIs(t, err, gemini.ErrTransient)
			assert.Equal(t, 1, rec.count(), "an unusable 2xx must not be retried")
		})
	}
}

// TestHTTPClient_GenerateJSON_NeverLeaksAPIKey is the security regression test:
// the key rides in the query string, so upstream error bodies and transport
// errors can quote it straight back at us.
func TestHTTPClient_GenerateJSON_NeverLeaksAPIKey(t *testing.T) {
	// echoKey answers with a body that contains the key it received, the way a
	// real provider does on an auth failure.
	echoKey := func(status int) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			key := r.URL.Query().Get("key")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_, _ = io.WriteString(w, `{"error":{"message":"API key not valid. key=`+key+` (passed in ?key=`+key+`)"}}`)
		}
	}

	cases := []struct {
		name       string
		status     int
		maxRetries int
	}{
		{name: "400 echoes the key back", status: http.StatusBadRequest, maxRetries: 3},
		{name: "403 echoes the key back", status: http.StatusForbidden, maxRetries: 3},
		{name: "500 echoes the key back after retries", status: http.StatusInternalServerError, maxRetries: 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client, rec := geServer(t, gemini.Config{MaxRetries: tc.maxRetries}, echoKey(tc.status))

			_, err := client.GenerateJSON(context.Background(), gemini.Request{Prompt: "go"})

			require.Error(t, err)
			require.Equal(t, geAPIKey, rec.last().key, "the fake provider must actually have seen the key")

			assert.NotContains(t, err.Error(), geAPIKey, "the API key must never appear in an error string")
			assert.Contains(t, err.Error(), "[REDACTED]", "the key must be replaced, not just dropped")
		})
	}

	t.Run("transport failure does not leak the key", func(t *testing.T) {
		// A server that is started and immediately closed gives a dial error whose
		// *url.Error wraps the full request URL, key included.
		srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		baseURL := srv.URL
		srv.Close()

		client := gemini.New(gemini.Config{
			APIKey:     geAPIKey,
			Model:      geModel,
			BaseURL:    baseURL,
			Timeout:    time.Second,
			MaxRetries: 1,
		})

		_, err := client.GenerateJSON(context.Background(), gemini.Request{Prompt: "go"})

		require.Error(t, err)
		assert.ErrorIs(t, err, gemini.ErrTransient)
		assert.NotContains(t, err.Error(), geAPIKey, "the API key must never appear in a transport error")
	})
}

func TestHTTPClient_GenerateJSON_ContextCancellation(t *testing.T) {
	t.Run("cancelling during backoff aborts instead of sleeping it out", func(t *testing.T) {
		served := make(chan struct{}, 16)

		client, rec := geServer(t, gemini.Config{MaxRetries: 5},
			func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = io.WriteString(w, `{"error":"boom"}`)
				served <- struct{}{}
			})

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		go func() {
			<-served
			cancel()
		}()

		start := time.Now()
		out, err := client.GenerateJSON(ctx, gemini.Request{Prompt: "go"})
		elapsed := time.Since(start)

		require.Error(t, err)
		assert.Nil(t, out)
		assert.ErrorIs(t, err, context.Canceled)
		// MaxRetries=5 would otherwise sleep 500ms + 1s + 2s + ... before giving up.
		assert.Less(t, elapsed, 400*time.Millisecond, "cancellation must not wait out the backoff")
		assert.LessOrEqual(t, rec.count(), 2, "no further attempts after cancellation")
	})

	t.Run("an already cancelled context never reaches the provider", func(t *testing.T) {
		client, rec := geServer(t, gemini.Config{MaxRetries: 2},
			geJSONHandler(http.StatusOK, geCandidates(`{"ok":true}`)))

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		out, err := client.GenerateJSON(ctx, gemini.Request{Prompt: "go"})

		require.Error(t, err)
		assert.Nil(t, out)
		assert.ErrorIs(t, err, context.Canceled)
		assert.Equal(t, 0, rec.count())
	})
}

func TestSchemaBuilders(t *testing.T) {
	t.Run("scalars use uppercase type names", func(t *testing.T) {
		cases := []struct {
			name   string
			schema map[string]any
			want   string
		}{
			{name: "Str", schema: gemini.Str("a string"), want: "STRING"},
			{name: "Num", schema: gemini.Num("a number"), want: "NUMBER"},
			{name: "Bool", schema: gemini.Bool("a flag"), want: "BOOLEAN"},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				assert.Equal(t, tc.want, tc.schema["type"])
				assert.Equal(t, strings.ToUpper(tc.want), tc.schema["type"], "type names must be uppercase")
			})
		}
	})

	t.Run("description is omitted when empty", func(t *testing.T) {
		withDesc := gemini.Str("explain yourself")
		assert.Equal(t, "explain yourself", withDesc["description"])

		withoutDesc := gemini.Str("")
		assert.NotContains(t, withoutDesc, "description")
	})

	t.Run("Object emits OBJECT with properties and optional required", func(t *testing.T) {
		props := map[string]any{"a": gemini.Str(""), "b": gemini.Num("")}

		withRequired := gemini.Object(props, "a", "b")
		assert.Equal(t, "OBJECT", withRequired["type"])
		assert.Equal(t, props, withRequired["properties"])
		assert.Equal(t, []string{"a", "b"}, withRequired["required"])

		withoutRequired := gemini.Object(props)
		assert.Equal(t, "OBJECT", withoutRequired["type"])
		assert.NotContains(t, withoutRequired, "required")

		nilProps := gemini.Object(nil)
		assert.Equal(t, "OBJECT", nilProps["type"])
		emptyProps, ok := nilProps["properties"].(map[string]any)
		require.True(t, ok, "nil properties must become an empty object, not a null")
		assert.Empty(t, emptyProps)
	})

	t.Run("Array emits ARRAY with items", func(t *testing.T) {
		items := gemini.Str("an entry")
		schema := gemini.Array(items)

		assert.Equal(t, "ARRAY", schema["type"])
		assert.Equal(t, items, schema["items"])
	})

	t.Run("Enum is a STRING with its values under enum", func(t *testing.T) {
		schema := gemini.Enum("the trend", "UP", "FLAT", "DOWN")

		assert.Equal(t, "STRING", schema["type"])
		assert.Equal(t, "the trend", schema["description"])
		assert.Equal(t, []string{"UP", "FLAT", "DOWN"}, schema["enum"])
		assert.NotContains(t, schema, "values", "enum values belong under the \"enum\" key")
	})

	t.Run("every type in a composite schema is uppercase after marshalling", func(t *testing.T) {
		schema := gemini.Object(map[string]any{
			"headline": gemini.Str("one line"),
			"score":    gemini.Num("0..100"),
			"cached":   gemini.Bool(""),
			"trend":    gemini.Enum("direction", "UP", "DOWN"),
			"weeks": gemini.Array(gemini.Object(map[string]any{
				"index": gemini.Num("1-based"),
				"tasks": gemini.Array(gemini.Str("")),
			}, "index")),
		}, "headline", "score")

		raw, err := json.Marshal(schema)
		require.NoError(t, err)

		var decoded any
		require.NoError(t, json.Unmarshal(raw, &decoded))

		allowed := map[string]bool{"OBJECT": true, "ARRAY": true, "STRING": true, "NUMBER": true, "BOOLEAN": true}
		seen := map[string]int{}

		var walk func(node any)
		walk = func(node any) {
			switch typed := node.(type) {
			case map[string]any:
				if kind, ok := typed["type"].(string); ok {
					assert.True(t, allowed[kind], "unexpected schema type %q", kind)
					seen[kind]++
				}
				for _, child := range typed {
					walk(child)
				}
			case []any:
				for _, child := range typed {
					walk(child)
				}
			}
		}
		walk(decoded)

		for kind := range allowed {
			assert.Positive(t, seen[kind], "composite schema should exercise %s", kind)
		}
	})
}

// Compile-time guard: the concrete client must keep satisfying the interface the
// AI service depends on.
var _ gemini.Client = (*gemini.HTTPClient)(nil)
