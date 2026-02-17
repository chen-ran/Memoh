package memory

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestLLMClientExtract(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"choices":[{"message":{"content":"{\"facts\":[\"hello\"]}"}}]}`))
	}))
	defer server.Close()

	client, err := NewLLMClient(nil, server.URL, "test-key", "gpt-4.1-nano-2025-04-14", 0)
	if err != nil {
		t.Fatalf("new llm client: %v", err)
	}
	resp, err := client.Extract(context.Background(), ExtractRequest{
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(resp.Facts) != 1 || resp.Facts[0] != "hello" {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestLLMClientTranslate(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"choices":[{"message":{"content":"{\"source_language\":\"en\",\"translations\":{\"es\":\"hola\",\"fr\":\"bonjour\"}}"}}]}`))
	}))
	defer server.Close()

	client, err := NewLLMClient(nil, server.URL, "test-key", "gpt-4.1-nano-2025-04-14", 0)
	if err != nil {
		t.Fatalf("new llm client: %v", err)
	}
	resp, err := client.Translate(context.Background(), TranslateRequest{
		Text:            "hello",
		TargetLanguages: []string{"es", "fr"},
	})
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if resp.SourceLanguage != "en" {
		t.Fatalf("unexpected source language: %s", resp.SourceLanguage)
	}
	if resp.Translations["es"] != "hola" || resp.Translations["fr"] != "bonjour" {
		t.Fatalf("unexpected translations: %+v", resp.Translations)
	}
}

type stubResolver struct {
	cfg ProviderConfig
	err error
}

func (s stubResolver) ResolveMemoryProvider(context.Context, string) (ProviderConfig, error) {
	if s.err != nil {
		return ProviderConfig{}, s.err
	}
	return s.cfg, nil
}

type stubLLM struct{}

func (stubLLM) Extract(context.Context, ExtractRequest) (ExtractResponse, error) {
	return ExtractResponse{}, nil
}
func (stubLLM) Decide(context.Context, DecideRequest) (DecideResponse, error) {
	return DecideResponse{}, nil
}
func (stubLLM) Compact(context.Context, CompactRequest) (CompactResponse, error) {
	return CompactResponse{}, nil
}
func (stubLLM) DetectLanguage(context.Context, string) (string, error) { return "en", nil }
func (stubLLM) Translate(context.Context, TranslateRequest) (TranslateResponse, error) {
	return TranslateResponse{Translations: map[string]string{"en": "hello"}}, nil
}

func TestDynamicLLMRejectsUnsupportedClientType(t *testing.T) {
	client := NewDynamicLLM(nil, stubResolver{
		cfg: ProviderConfig{
			ModelID:    "claude-3-5-sonnet",
			BaseURL:    "https://example.com",
			APIKey:     "k",
			ClientType: "bedrock",
		},
	}, 0, func(_ *slog.Logger, _ ProviderConfig, _ time.Duration) (LLM, error) {
		return stubLLM{}, nil
	})

	if _, err := client.DetectLanguage(context.Background(), "hello"); err == nil {
		t.Fatalf("expected unsupported client type error")
	}
}

func TestDynamicLLMDelegatesToFactory(t *testing.T) {
	called := false
	client := NewDynamicLLM(nil, stubResolver{
		cfg: ProviderConfig{
			ModelID:    "gpt-4.1",
			BaseURL:    "https://example.com",
			APIKey:     "k",
			ClientType: "openai",
		},
	}, 5*time.Second, func(_ *slog.Logger, cfg ProviderConfig, timeout time.Duration) (LLM, error) {
		called = true
		if cfg.ModelID != "gpt-4.1" || timeout != 5*time.Second {
			t.Fatalf("unexpected cfg=%+v timeout=%v", cfg, timeout)
		}
		return stubLLM{}, nil
	})

	lang, err := client.DetectLanguage(context.Background(), "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lang != "en" || !called {
		t.Fatalf("unexpected result lang=%s called=%v", lang, called)
	}
}

func TestDynamicLLMResolverError(t *testing.T) {
	client := NewDynamicLLM(nil, stubResolver{err: errors.New("boom")}, 0, nil)
	if _, err := client.DetectLanguage(context.Background(), "hello"); err == nil {
		t.Fatalf("expected resolver error")
	}
}
