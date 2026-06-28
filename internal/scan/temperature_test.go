package scan

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func fptr(f float64) *float64 { return &f }

func TestResolveTemperaturePrecedence(t *testing.T) {
	t.Setenv("AURSCAN_OPENAI_TEMPERATURE", "")
	if got := resolveTemperature(Backend{}); got != defaultTemperature {
		t.Fatalf("default: got %v want %v", got, defaultTemperature)
	}
	t.Setenv("AURSCAN_OPENAI_TEMPERATURE", "0.7")
	if got := resolveTemperature(Backend{}); got != 0.7 {
		t.Fatalf("env: got %v want 0.7", got)
	}
	if got := resolveTemperature(Backend{Temperature: fptr(1.0)}); got != 1.0 {
		t.Fatalf("per-backend must win over env: got %v want 1.0", got)
	}
	t.Setenv("AURSCAN_OPENAI_TEMPERATURE", "garbage")
	if got := resolveTemperature(Backend{}); got != defaultTemperature {
		t.Fatalf("invalid env should fall back to default: got %v", got)
	}
	// temperature=0 is a valid explicit value, not "unset"
	if got := resolveTemperature(Backend{Temperature: fptr(0)}); got != 0 {
		t.Fatalf("explicit 0 must be honoured: got %v", got)
	}
}

func TestResolveMaxTokensPrecedence(t *testing.T) {
	t.Setenv("AURSCAN_OPENAI_MAX_TOKENS", "")
	if got := resolveMaxTokens(Backend{}); got != maxOutTokens {
		t.Fatalf("default: got %d want %d", got, maxOutTokens)
	}
	t.Setenv("AURSCAN_OPENAI_MAX_TOKENS", "32000")
	if got := resolveMaxTokens(Backend{}); got != 32000 {
		t.Fatalf("env: got %d want 32000", got)
	}
	if got := resolveMaxTokens(Backend{MaxTokens: 9000}); got != 9000 {
		t.Fatalf("per-backend must win: got %d want 9000", got)
	}
}

// TestOpenAIRequestCarriesConfig asserts the values actually reach the wire.
func TestOpenAIRequestCarriesConfig(t *testing.T) {
	var gotTemp float64
	var gotMax float64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		gotTemp, _ = m["temperature"].(float64)
		gotMax, _ = m["max_tokens"].(float64)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, openAIEnvelope(`{"verdict":"OK"}`))
	}))
	defer srv.Close()
	be := Backend{Kind: "openai", URL: srv.URL, Temperature: fptr(1.0), MaxTokens: 40000}
	_, _, err := callOpenAI(context.Background(), 5*time.Second, be, "sys", "body", 1)
	if err != nil {
		t.Fatalf("callOpenAI: %v", err)
	}
	if gotTemp != 1.0 {
		t.Fatalf("temperature on wire = %v, want 1.0", gotTemp)
	}
	if gotMax != 40000 {
		t.Fatalf("max_tokens on wire = %v, want 40000", gotMax)
	}
}

// TestOpenAIEmptyContentActionable: a reasoning model that returns finish_reason
// "length" with empty content yields an actionable error, not a silent pass.
func TestOpenAIEmptyContentActionable(t *testing.T) {
	body, _ := json.Marshal(map[string]any{
		"choices": []map[string]any{{
			"finish_reason": "length",
			"message":       map[string]string{"content": "", "reasoning_content": "thinking..."},
		}},
	})
	var c int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&c, 1)
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	defer srv.Close()
	_, _, err := callOpenAI(context.Background(), 5*time.Second, Backend{Kind: "openai", URL: srv.URL}, "s", "b", 1)
	if err == nil {
		t.Fatal("expected an error for empty content, got nil")
	}
	for _, want := range []string{"max_tokens", "temperature=1.0", "finish_reason"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q missing %q", err.Error(), want)
		}
	}
}

// TestBackendsFromConfigParsesTuning checks llmN.conf temperature/max_tokens.
func TestBackendsFromConfigParsesTuning(t *testing.T) {
	bs := BackendsFromConfig([]map[string]string{
		{"backend": "openai", "url": "http://x", "temperature": "1.0", "max_tokens": "32000"},
	})
	if len(bs) != 1 {
		t.Fatalf("got %d backends", len(bs))
	}
	if bs[0].Temperature == nil || *bs[0].Temperature != 1.0 {
		t.Fatalf("temperature not parsed: %+v", bs[0].Temperature)
	}
	if bs[0].MaxTokens != 32000 {
		t.Fatalf("max_tokens not parsed: %d", bs[0].MaxTokens)
	}
}
