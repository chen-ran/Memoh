package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/go-kratos/blades"
	bladesanthropic "github.com/go-kratos/blades/contrib/anthropic"
	bladesgemini "github.com/go-kratos/blades/contrib/gemini"
	bladesopenai "github.com/go-kratos/blades/contrib/openai"
	"google.golang.org/genai"
)

// ProviderConfig describes provider/model settings for memory LLM calls.
type ProviderConfig struct {
	ModelID    string
	BaseURL    string
	APIKey     string
	ClientType string
}

// ProviderResolver resolves provider config for a bot at runtime.
type ProviderResolver interface {
	ResolveMemoryProvider(ctx context.Context, botID string) (ProviderConfig, error)
}

// LLMClientFactory creates a concrete memory LLM client from provider config.
type LLMClientFactory func(log *slog.Logger, cfg ProviderConfig, timeout time.Duration) (LLM, error)

// DynamicLLM resolves provider/model at call time then delegates to concrete client.
type DynamicLLM struct {
	resolver ProviderResolver
	factory  LLMClientFactory
	timeout  time.Duration
	logger   *slog.Logger
}

// NewDynamicLLM creates a DI-friendly lazy memory LLM gateway.
func NewDynamicLLM(log *slog.Logger, resolver ProviderResolver, timeout time.Duration, factory LLMClientFactory) *DynamicLLM {
	if log == nil {
		log = slog.Default()
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	if factory == nil {
		factory = NewLLMClientForProvider
	}
	return &DynamicLLM{
		resolver: resolver,
		factory:  factory,
		timeout:  timeout,
		logger:   log.With(slog.String("client", "memory_dynamic_llm")),
	}
}

func (d *DynamicLLM) Extract(ctx context.Context, req ExtractRequest) (ExtractResponse, error) {
	client, err := d.resolve(ctx)
	if err != nil {
		return ExtractResponse{}, err
	}
	return client.Extract(ctx, req)
}

func (d *DynamicLLM) Decide(ctx context.Context, req DecideRequest) (DecideResponse, error) {
	client, err := d.resolve(ctx)
	if err != nil {
		return DecideResponse{}, err
	}
	return client.Decide(ctx, req)
}

func (d *DynamicLLM) Compact(ctx context.Context, req CompactRequest) (CompactResponse, error) {
	client, err := d.resolve(ctx)
	if err != nil {
		return CompactResponse{}, err
	}
	return client.Compact(ctx, req)
}

func (d *DynamicLLM) DetectLanguage(ctx context.Context, text string) (string, error) {
	client, err := d.resolve(ctx)
	if err != nil {
		return "", err
	}
	return client.DetectLanguage(ctx, text)
}

func (d *DynamicLLM) Translate(ctx context.Context, req TranslateRequest) (TranslateResponse, error) {
	client, err := d.resolve(ctx)
	if err != nil {
		return TranslateResponse{}, err
	}
	return client.Translate(ctx, req)
}

func (d *DynamicLLM) resolve(ctx context.Context) (LLM, error) {
	if d.resolver == nil {
		return nil, fmt.Errorf("memory provider resolver not configured")
	}
	cfg, err := d.resolver.ResolveMemoryProvider(ctx, BotIDFromContext(ctx))
	if err != nil {
		return nil, err
	}
	clientType := strings.ToLower(strings.TrimSpace(cfg.ClientType))
	switch clientType {
	case "openai", "openai-compat", "anthropic", "google":
	default:
		return nil, fmt.Errorf("memory provider client type not supported: %s", cfg.ClientType)
	}
	return d.factory(d.logger, cfg, d.timeout)
}

type llmClient struct {
	provider blades.ModelProvider
	logger   *slog.Logger
}

// NewLLMClient keeps the old constructor shape and builds an OpenAI-compatible client.
func NewLLMClient(log *slog.Logger, baseURL, apiKey, model string, timeout time.Duration) (LLM, error) {
	return NewLLMClientForProvider(log, ProviderConfig{
		ModelID:    model,
		BaseURL:    baseURL,
		APIKey:     apiKey,
		ClientType: "openai-compat",
	}, timeout)
}

// NewLLMClientForProvider builds a memory LLM client backed by Blades providers.
func NewLLMClientForProvider(log *slog.Logger, cfg ProviderConfig, _ time.Duration) (LLM, error) {
	if log == nil {
		log = slog.Default()
	}
	modelID := strings.TrimSpace(cfg.ModelID)
	if modelID == "" {
		return nil, fmt.Errorf("llm client: model is required")
	}
	clientType := strings.ToLower(strings.TrimSpace(cfg.ClientType))
	baseURL := strings.TrimSpace(cfg.BaseURL)
	apiKey := strings.TrimSpace(cfg.APIKey)

	var provider blades.ModelProvider
	switch clientType {
	case "openai", "openai-compat":
		provider = bladesopenai.NewModel(modelID, bladesopenai.Config{
			BaseURL:     baseURL,
			APIKey:      apiKey,
			Temperature: 0,
		})
	case "anthropic":
		provider = bladesanthropic.NewModel(modelID, bladesanthropic.Config{
			BaseURL:     baseURL,
			APIKey:      apiKey,
			Temperature: 0,
		})
	case "google":
		gcfg := bladesgemini.Config{
			ClientConfig: genai.ClientConfig{
				APIKey:  apiKey,
				Backend: genai.BackendGeminiAPI,
			},
		}
		if baseURL != "" {
			gcfg.ClientConfig.HTTPOptions = genai.HTTPOptions{BaseURL: baseURL}
		}
		p, err := bladesgemini.NewModel(context.Background(), modelID, gcfg)
		if err != nil {
			return nil, err
		}
		provider = p
	default:
		return nil, fmt.Errorf("memory provider client type not supported: %s", cfg.ClientType)
	}

	return &llmClient{
		provider: provider,
		logger:   log.With(slog.String("client", "memory_llm")),
	}, nil
}

func (c *llmClient) Extract(ctx context.Context, req ExtractRequest) (ExtractResponse, error) {
	if len(req.Messages) == 0 {
		return ExtractResponse{}, fmt.Errorf("messages is required")
	}
	parsedMessages := strings.Join(formatMessages(req.Messages), "\n")
	systemPrompt, userPrompt := getFactRetrievalMessages(parsedMessages)
	content, err := c.callChat(ctx, []chatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	})
	if err != nil {
		return ExtractResponse{}, err
	}
	var parsed ExtractResponse
	if err := jsonUnmarshalText(content, &parsed); err != nil {
		return ExtractResponse{}, err
	}
	return parsed, nil
}

func (c *llmClient) Decide(ctx context.Context, req DecideRequest) (DecideResponse, error) {
	if len(req.Facts) == 0 {
		return DecideResponse{}, fmt.Errorf("facts is required")
	}
	retrieved := make([]map[string]string, 0, len(req.Candidates))
	for _, candidate := range req.Candidates {
		retrieved = append(retrieved, map[string]string{
			"id":   candidate.ID,
			"text": candidate.Memory,
		})
	}
	prompt := getUpdateMemoryMessages(retrieved, req.Facts)
	content, err := c.callChat(ctx, []chatMessage{
		{Role: "user", Content: prompt},
	})
	if err != nil {
		return DecideResponse{}, err
	}

	cleaned := removeCodeBlocks(content)
	var memoryItems []map[string]any
	var raw map[string]any
	if err := json.Unmarshal([]byte(cleaned), &raw); err == nil {
		memoryItems = normalizeMemoryItems(raw["memory"])
	} else {
		var arr []any
		if err := json.Unmarshal([]byte(cleaned), &arr); err != nil {
			return DecideResponse{}, fmt.Errorf("failed to parse LLM response: %w", err)
		}
		memoryItems = normalizeMemoryItems(arr)
	}

	actions := make([]DecisionAction, 0, len(memoryItems))
	for _, item := range memoryItems {
		event := strings.ToUpper(asString(item["event"]))
		if event == "" {
			event = "ADD"
		}
		if event == "NONE" {
			continue
		}
		text := asString(item["text"])
		if text == "" {
			text = asString(item["fact"])
		}
		if strings.TrimSpace(text) == "" {
			continue
		}
		actions = append(actions, DecisionAction{
			Event:     event,
			ID:        asString(item["id"]),
			Text:      text,
			OldMemory: asString(item["old_memory"]),
		})
	}
	return DecideResponse{Actions: actions}, nil
}

func (c *llmClient) Compact(ctx context.Context, req CompactRequest) (CompactResponse, error) {
	if len(req.Memories) == 0 {
		return CompactResponse{}, fmt.Errorf("memories is required")
	}
	memories := make([]map[string]string, 0, len(req.Memories))
	for _, m := range req.Memories {
		entry := map[string]string{
			"id":   m.ID,
			"text": m.Memory,
		}
		if m.CreatedAt != "" {
			entry["created_at"] = m.CreatedAt
		}
		memories = append(memories, entry)
	}
	systemPrompt, userPrompt := getCompactMemoryMessages(memories, req.TargetCount, req.DecayDays)
	content, err := c.callChat(ctx, []chatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	})
	if err != nil {
		return CompactResponse{}, err
	}
	var parsed CompactResponse
	if err := jsonUnmarshalText(content, &parsed); err != nil {
		return CompactResponse{}, fmt.Errorf("failed to parse compact response: %w", err)
	}
	return parsed, nil
}

func (c *llmClient) DetectLanguage(ctx context.Context, text string) (string, error) {
	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("text is required")
	}
	systemPrompt, userPrompt := getLanguageDetectionMessages(text)
	content, err := c.callChat(ctx, []chatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	})
	if err != nil {
		return "", err
	}
	var parsed struct {
		Language string `json:"language"`
	}
	if err := jsonUnmarshalText(content, &parsed); err != nil {
		return "", err
	}
	lang := strings.ToLower(strings.TrimSpace(parsed.Language))
	if !isAllowedLanguageCode(lang) {
		return "", fmt.Errorf("unsupported language code: %s", lang)
	}
	return lang, nil
}

func (c *llmClient) Translate(ctx context.Context, req TranslateRequest) (TranslateResponse, error) {
	text := strings.TrimSpace(req.Text)
	if text == "" {
		return TranslateResponse{}, fmt.Errorf("text is required")
	}
	if len(req.TargetLanguages) == 0 {
		return TranslateResponse{}, fmt.Errorf("target_languages is required")
	}
	targets := make([]string, 0, len(req.TargetLanguages))
	seen := make(map[string]struct{}, len(req.TargetLanguages))
	for _, lang := range req.TargetLanguages {
		normalized := strings.ToLower(strings.TrimSpace(lang))
		if normalized == "" {
			continue
		}
		if !isAllowedLanguageCode(normalized) {
			return TranslateResponse{}, fmt.Errorf("unsupported target language code: %s", lang)
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		targets = append(targets, normalized)
	}
	if len(targets) == 0 {
		return TranslateResponse{}, fmt.Errorf("target_languages is required")
	}

	systemPrompt, userPrompt := getTranslationMessages(text, targets)
	content, err := c.callChat(ctx, []chatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	})
	if err != nil {
		return TranslateResponse{}, err
	}
	var parsed TranslateResponse
	if err := jsonUnmarshalText(content, &parsed); err != nil {
		return TranslateResponse{}, fmt.Errorf("failed to parse translation response: %w", err)
	}
	if parsed.Translations == nil {
		parsed.Translations = map[string]string{}
	}
	for _, target := range targets {
		if strings.TrimSpace(parsed.Translations[target]) == "" {
			return TranslateResponse{}, fmt.Errorf("translation missing for language: %s", target)
		}
	}
	if parsed.SourceLanguage != "" {
		parsed.SourceLanguage = strings.ToLower(strings.TrimSpace(parsed.SourceLanguage))
		if !isAllowedLanguageCode(parsed.SourceLanguage) {
			parsed.SourceLanguage = ""
		}
	}
	return parsed, nil
}

func (c *llmClient) callChat(ctx context.Context, messages []chatMessage) (string, error) {
	if c.provider == nil {
		return "", fmt.Errorf("llm provider not configured")
	}
	req := &blades.ModelRequest{
		Messages: toBladesMessages(messages),
	}
	resp, err := c.provider.Generate(ctx, req)
	if err != nil {
		return "", err
	}
	if resp == nil || resp.Message == nil {
		return "", fmt.Errorf("llm response missing content")
	}
	content := strings.TrimSpace(resp.Message.Text())
	if content == "" {
		return "", fmt.Errorf("llm response missing content")
	}
	return content, nil
}

func toBladesMessages(messages []chatMessage) []*blades.Message {
	out := make([]*blades.Message, 0, len(messages))
	for _, msg := range messages {
		switch strings.ToLower(strings.TrimSpace(msg.Role)) {
		case "system":
			out = append(out, blades.SystemMessage(msg.Content))
		case "assistant":
			out = append(out, blades.AssistantMessage(msg.Content))
		default:
			out = append(out, blades.UserMessage(msg.Content))
		}
	}
	return out
}

func jsonUnmarshalText(content string, out any) error {
	return json.Unmarshal([]byte(removeCodeBlocks(content)), out)
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func formatMessages(messages []Message) []string {
	formatted := make([]string, 0, len(messages))
	for _, message := range messages {
		formatted = append(formatted, fmt.Sprintf("%s: %s", message.Role, message.Content))
	}
	return formatted
}

func asString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case float64:
		if typed == float64(int64(typed)) {
			return fmt.Sprintf("%d", int64(typed))
		}
		return fmt.Sprintf("%f", typed)
	case int:
		return fmt.Sprintf("%d", typed)
	case int64:
		return fmt.Sprintf("%d", typed)
	default:
		return ""
	}
}

func normalizeMemoryItems(value any) []map[string]any {
	switch typed := value.(type) {
	case []any:
		items := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			if m, ok := item.(map[string]any); ok {
				items = append(items, m)
			}
		}
		return items
	case map[string]any:
		if _, hasText := typed["text"]; hasText {
			return []map[string]any{typed}
		}
		if _, hasFact := typed["fact"]; hasFact {
			return []map[string]any{typed}
		}
		if _, hasEvent := typed["event"]; hasEvent {
			return []map[string]any{typed}
		}
		items := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			if m, ok := item.(map[string]any); ok {
				items = append(items, m)
			}
		}
		return items
	default:
		return nil
	}
}

func isAllowedLanguageCode(code string) bool {
	switch strings.ToLower(strings.TrimSpace(code)) {
	case "ar", "bg", "ca", "cjk", "ckb", "da", "de", "el", "en", "es", "eu",
		"fa", "fi", "fr", "ga", "gl", "hi", "hr", "hu", "hy", "id", "in",
		"it", "nl", "no", "pl", "pt", "ro", "ru", "sv", "tr":
		return true
	default:
		return false
	}
}
