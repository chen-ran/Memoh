package builtin

import (
	"log/slog"
	"strconv"
	"strings"

	"github.com/memohai/memoh/internal/config"
	adapters "github.com/memohai/memoh/internal/memory/adapters"
)

// BuiltinMemoryMode represents the operating mode of the built-in memory provider.
type BuiltinMemoryMode string

const (
	ModeOff    BuiltinMemoryMode = "off"
	ModeSparse BuiltinMemoryMode = "sparse"
	ModeDense  BuiltinMemoryMode = "dense"
)

// NewBuiltinRuntimeFromConfig returns the appropriate memoryRuntime based on the
// provider's persisted config (memory_mode field). Falls back to the file runtime for "off" or unknown.
func NewBuiltinRuntimeFromConfig(log *slog.Logger, providerConfig map[string]any, fileRuntime any, cfg config.Config) (any, error) {
	mode := BuiltinMemoryMode(strings.TrimSpace(adapters.StringFromConfig(providerConfig, "memory_mode")))

	switch mode {
	case ModeSparse:
		host, port := parseQdrantHostPort(cfg.Qdrant.BaseURL)
		if host == "" {
			host = "localhost"
		}
		if port == 0 {
			port = 6334
		}
		collection := adapters.StringFromConfig(providerConfig, "qdrant_collection")
		if collection == "" {
			collection = "memory_sparse"
		}
		rt, err := newSparseRuntime(host, port, cfg.Qdrant.APIKey, collection)
		if err != nil {
			if log != nil {
				log.Warn("sparse runtime init failed, falling back to file runtime", slog.Any("error", err))
			}
			return fileRuntime, nil
		}
		return rt, nil

	case ModeDense:
		rt, err := newDenseRuntime(providerConfig)
		if err != nil {
			if log != nil {
				log.Warn("dense runtime init failed, falling back to file runtime", slog.Any("error", err))
			}
			return fileRuntime, nil
		}
		return rt, nil

	default:
		return fileRuntime, nil
	}
}

// parseQdrantHostPort extracts host and gRPC port from a Qdrant base URL.
// Qdrant base URLs are typically HTTP (port 6333), but the gRPC port is 6334.
func parseQdrantHostPort(baseURL string) (string, int) {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return "", 0
	}
	baseURL = strings.TrimPrefix(baseURL, "http://")
	baseURL = strings.TrimPrefix(baseURL, "https://")
	parts := strings.SplitN(baseURL, ":", 2)
	host := parts[0]
	if len(parts) == 2 {
		httpPort, err := strconv.Atoi(strings.TrimRight(parts[1], "/"))
		if err == nil {
			// gRPC port is typically HTTP port + 1 (6333 → 6334)
			return host, httpPort + 1
		}
	}
	return host, 6334
}
