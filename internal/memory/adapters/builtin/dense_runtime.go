package builtin

import (
	"context"
	"fmt"
	"strings"

	adapters "github.com/memohai/memoh/internal/memory/adapters"
	"github.com/memohai/memoh/internal/memory/adapters/mem0"
)

// denseRuntime implements memoryRuntime by delegating to a local mem0 service.
// The mem0 service handles embedding (via a configured third-party embedding model)
// and rerank internally, using Qdrant as its vector store.
type denseRuntime struct {
	provider *mem0.Mem0Provider
}

func newDenseRuntime(config map[string]any) (*denseRuntime, error) {
	baseURL := adapters.StringFromConfig(config, "mem0_base_url")
	if strings.TrimSpace(baseURL) == "" {
		return nil, fmt.Errorf("dense runtime: mem0_base_url is required")
	}
	// Pass a mem0-compatible config to the existing mem0 provider constructor.
	mem0Config := map[string]any{
		"base_url":   baseURL,
		"api_key":    adapters.StringFromConfig(config, "mem0_api_key"),
		"org_id":     adapters.StringFromConfig(config, "mem0_org_id"),
		"project_id": adapters.StringFromConfig(config, "mem0_project_id"),
	}
	provider, err := mem0.NewMem0Provider(nil, mem0Config)
	if err != nil {
		return nil, fmt.Errorf("dense runtime: %w", err)
	}
	return &denseRuntime{provider: provider}, nil
}

func (r *denseRuntime) Add(ctx context.Context, req adapters.AddRequest) (adapters.SearchResponse, error) {
	return r.provider.Add(ctx, req)
}

func (r *denseRuntime) Search(ctx context.Context, req adapters.SearchRequest) (adapters.SearchResponse, error) {
	return r.provider.Search(ctx, req)
}

func (r *denseRuntime) GetAll(ctx context.Context, req adapters.GetAllRequest) (adapters.SearchResponse, error) {
	return r.provider.GetAll(ctx, req)
}

func (r *denseRuntime) Update(ctx context.Context, req adapters.UpdateRequest) (adapters.MemoryItem, error) {
	return r.provider.Update(ctx, req)
}

func (r *denseRuntime) Delete(ctx context.Context, memoryID string) (adapters.DeleteResponse, error) {
	return r.provider.Delete(ctx, memoryID)
}

func (r *denseRuntime) DeleteBatch(ctx context.Context, memoryIDs []string) (adapters.DeleteResponse, error) {
	return r.provider.DeleteBatch(ctx, memoryIDs)
}

func (r *denseRuntime) DeleteAll(ctx context.Context, req adapters.DeleteAllRequest) (adapters.DeleteResponse, error) {
	return r.provider.DeleteAll(ctx, req)
}

func (r *denseRuntime) Compact(ctx context.Context, filters map[string]any, ratio float64, decayDays int) (adapters.CompactResult, error) {
	return r.provider.Compact(ctx, filters, ratio, decayDays)
}

func (r *denseRuntime) Usage(ctx context.Context, filters map[string]any) (adapters.UsageResponse, error) {
	return r.provider.Usage(ctx, filters)
}
