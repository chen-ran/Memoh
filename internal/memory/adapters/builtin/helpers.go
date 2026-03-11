package builtin

import (
	"fmt"
	"strings"
)

// runtimeBotID extracts the bot ID from the request or filters.
func runtimeBotID(botID string, filters map[string]any) (string, error) {
	botID = strings.TrimSpace(botID)
	if botID == "" {
		botID = strings.TrimSpace(anyString(filters, "bot_id"))
	}
	if botID == "" {
		botID = strings.TrimSpace(anyString(filters, "scopeId"))
	}
	if botID == "" {
		return "", fmt.Errorf("bot_id is required")
	}
	return botID, nil
}

func anyString(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	return fmt.Sprint(v)
}
