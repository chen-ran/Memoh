package builtin

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	adapters "github.com/memohai/memoh/internal/memory/adapters"
	qdrantclient "github.com/memohai/memoh/internal/memory/qdrant"
	"github.com/memohai/memoh/internal/memory/sparse"
)

// sparseRuntime implements memoryRuntime using:
//   - The Python sparse encoding service (OpenSearch neural sparse model) for text → sparse vector
//   - Qdrant (official go-client gRPC SDK) for vector storage and search
type sparseRuntime struct {
	qdrant  *qdrantclient.Client
	encoder *sparse.Client
}

func newSparseRuntime(qdrantHost string, qdrantPort int, qdrantAPIKey, collection string) (*sparseRuntime, error) {
	if strings.TrimSpace(qdrantHost) == "" {
		return nil, errors.New("sparse runtime: qdrant host is required")
	}
	qClient, err := qdrantclient.NewClient(qdrantHost, qdrantPort, qdrantAPIKey, collection)
	if err != nil {
		return nil, fmt.Errorf("sparse runtime: %w", err)
	}
	return &sparseRuntime{
		qdrant:  qClient,
		encoder: sparse.NewClient("http://127.0.0.1:8085"),
	}, nil
}

func (r *sparseRuntime) ensureCollection(ctx context.Context) error {
	return r.qdrant.EnsureCollection(ctx)
}

func (r *sparseRuntime) Add(ctx context.Context, req adapters.AddRequest) (adapters.SearchResponse, error) {
	botID, err := runtimeBotID(req.BotID, req.Filters)
	if err != nil {
		return adapters.SearchResponse{}, err
	}
	if err := r.ensureCollection(ctx); err != nil {
		return adapters.SearchResponse{}, err
	}

	text := strings.TrimSpace(req.Message)
	if text == "" && len(req.Messages) > 0 {
		var parts []string
		for _, m := range req.Messages {
			if c := strings.TrimSpace(m.Content); c != "" {
				role := strings.ToUpper(strings.TrimSpace(m.Role))
				if role == "" {
					role = "MESSAGE"
				}
				parts = append(parts, "["+role+"] "+c)
			}
		}
		text = strings.Join(parts, "\n")
	}
	if text == "" {
		return adapters.SearchResponse{}, errors.New("sparse runtime: message is required")
	}

	now := time.Now().UTC().Format(time.RFC3339)
	id := uuid.New().String()
	vec, err := r.encoder.EncodeDocument(ctx, text)
	if err != nil {
		return adapters.SearchResponse{}, fmt.Errorf("sparse encode document: %w", err)
	}

	err = r.qdrant.Upsert(ctx, id, qdrantclient.SparseVector{
		Indices: vec.Indices,
		Values:  vec.Values,
	}, map[string]string{
		"memory":     text,
		"bot_id":     botID,
		"created_at": now,
		"updated_at": now,
	})
	if err != nil {
		return adapters.SearchResponse{}, err
	}

	item := adapters.MemoryItem{
		ID:        id,
		Memory:    text,
		BotID:     botID,
		CreatedAt: now,
		UpdatedAt: now,
	}
	return adapters.SearchResponse{Results: []adapters.MemoryItem{item}}, nil
}

func (r *sparseRuntime) Search(ctx context.Context, req adapters.SearchRequest) (adapters.SearchResponse, error) {
	botID, err := runtimeBotID(req.BotID, req.Filters)
	if err != nil {
		return adapters.SearchResponse{}, err
	}
	if err := r.ensureCollection(ctx); err != nil {
		return adapters.SearchResponse{}, err
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}

	vec, err := r.encoder.EncodeQuery(ctx, req.Query)
	if err != nil {
		return adapters.SearchResponse{}, fmt.Errorf("sparse encode query: %w", err)
	}
	results, err := r.qdrant.Search(ctx, qdrantclient.SparseVector{
		Indices: vec.Indices,
		Values:  vec.Values,
	}, botID, limit)
	if err != nil {
		return adapters.SearchResponse{}, err
	}
	items := make([]adapters.MemoryItem, 0, len(results))
	for _, r := range results {
		items = append(items, sparseResultToItem(r))
	}
	return adapters.SearchResponse{Results: items}, nil
}

func (r *sparseRuntime) GetAll(ctx context.Context, req adapters.GetAllRequest) (adapters.SearchResponse, error) {
	botID, err := runtimeBotID(req.BotID, req.Filters)
	if err != nil {
		return adapters.SearchResponse{}, err
	}
	if err := r.ensureCollection(ctx); err != nil {
		return adapters.SearchResponse{}, err
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 1000
	}

	results, err := r.qdrant.Scroll(ctx, botID, limit)
	if err != nil {
		return adapters.SearchResponse{}, err
	}

	items := make([]adapters.MemoryItem, 0, len(results))
	for _, r := range results {
		items = append(items, sparseResultToItem(r))
	}
	sort.Slice(items, func(i, j int) bool { return items[i].UpdatedAt > items[j].UpdatedAt })
	return adapters.SearchResponse{Results: items}, nil
}

func (r *sparseRuntime) Update(ctx context.Context, req adapters.UpdateRequest) (adapters.MemoryItem, error) {
	memoryID := strings.TrimSpace(req.MemoryID)
	if memoryID == "" {
		return adapters.MemoryItem{}, errors.New("sparse runtime: memory_id is required")
	}
	text := strings.TrimSpace(req.Memory)
	if text == "" {
		return adapters.MemoryItem{}, errors.New("sparse runtime: memory is required")
	}
	if err := r.ensureCollection(ctx); err != nil {
		return adapters.MemoryItem{}, err
	}

	existing, err := r.qdrant.GetByID(ctx, memoryID)
	if err != nil {
		return adapters.MemoryItem{}, err
	}
	var createdAt, botID string
	if existing != nil && existing.Payload != nil {
		createdAt = existing.Payload["created_at"]
		botID = existing.Payload["bot_id"]
	}
	if createdAt == "" {
		createdAt = time.Now().UTC().Format(time.RFC3339)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	vec, encErr := r.encoder.EncodeDocument(ctx, text)
	if encErr != nil {
		return adapters.MemoryItem{}, fmt.Errorf("sparse encode document: %w", encErr)
	}
	err = r.qdrant.Upsert(ctx, memoryID, qdrantclient.SparseVector{
		Indices: vec.Indices,
		Values:  vec.Values,
	}, map[string]string{
		"memory":     text,
		"bot_id":     botID,
		"created_at": createdAt,
		"updated_at": now,
	})
	if err != nil {
		return adapters.MemoryItem{}, err
	}

	return adapters.MemoryItem{
		ID:        memoryID,
		Memory:    text,
		BotID:     botID,
		CreatedAt: createdAt,
		UpdatedAt: now,
	}, nil
}

func (r *sparseRuntime) Delete(ctx context.Context, memoryID string) (adapters.DeleteResponse, error) {
	return r.DeleteBatch(ctx, []string{memoryID})
}

func (r *sparseRuntime) DeleteBatch(ctx context.Context, memoryIDs []string) (adapters.DeleteResponse, error) {
	if err := r.ensureCollection(ctx); err != nil {
		return adapters.DeleteResponse{}, err
	}
	if err := r.qdrant.DeleteByIDs(ctx, memoryIDs); err != nil {
		return adapters.DeleteResponse{}, err
	}
	return adapters.DeleteResponse{Message: "Memories deleted successfully!"}, nil
}

func (r *sparseRuntime) DeleteAll(ctx context.Context, req adapters.DeleteAllRequest) (adapters.DeleteResponse, error) {
	botID, err := runtimeBotID(req.BotID, req.Filters)
	if err != nil {
		return adapters.DeleteResponse{}, err
	}
	if err := r.ensureCollection(ctx); err != nil {
		return adapters.DeleteResponse{}, err
	}
	if err := r.qdrant.DeleteByBotID(ctx, botID); err != nil {
		return adapters.DeleteResponse{}, err
	}
	return adapters.DeleteResponse{Message: "All memories deleted successfully!"}, nil
}

func (r *sparseRuntime) Compact(ctx context.Context, filters map[string]any, ratio float64, _ int) (adapters.CompactResult, error) {
	botID, err := runtimeBotID("", filters)
	if err != nil {
		return adapters.CompactResult{}, err
	}
	if err := r.ensureCollection(ctx); err != nil {
		return adapters.CompactResult{}, err
	}

	all, err := r.qdrant.Scroll(ctx, botID, 10000)
	if err != nil {
		return adapters.CompactResult{}, err
	}
	before := len(all)
	if before == 0 {
		return adapters.CompactResult{Ratio: ratio}, nil
	}

	sort.Slice(all, func(i, j int) bool {
		return all[i].Payload["updated_at"] > all[j].Payload["updated_at"]
	})
	target := int(float64(before) * ratio)
	if target < 1 {
		target = 1
	}
	if target > before {
		target = before
	}
	toDrop := all[target:]
	ids := make([]string, 0, len(toDrop))
	for _, p := range toDrop {
		ids = append(ids, p.ID)
	}
	if len(ids) > 0 {
		if err := r.qdrant.DeleteByIDs(ctx, ids); err != nil {
			return adapters.CompactResult{}, err
		}
	}

	kept := make([]adapters.MemoryItem, 0, target)
	for _, p := range all[:target] {
		kept = append(kept, sparseResultToItem(p))
	}
	return adapters.CompactResult{
		BeforeCount: before,
		AfterCount:  len(kept),
		Ratio:       ratio,
		Results:     kept,
	}, nil
}

func (r *sparseRuntime) Usage(ctx context.Context, filters map[string]any) (adapters.UsageResponse, error) {
	botID, err := runtimeBotID("", filters)
	if err != nil {
		return adapters.UsageResponse{}, err
	}
	if err := r.ensureCollection(ctx); err != nil {
		return adapters.UsageResponse{}, err
	}

	count, err := r.qdrant.Count(ctx, botID)
	if err != nil {
		return adapters.UsageResponse{}, err
	}
	return adapters.UsageResponse{Count: count}, nil
}

// --- helpers ---

func sparseResultToItem(r qdrantclient.SearchResult) adapters.MemoryItem {
	item := adapters.MemoryItem{
		ID:    r.ID,
		Score: r.Score,
	}
	if r.Payload != nil {
		item.Memory = r.Payload["memory"]
		item.BotID = r.Payload["bot_id"]
		item.CreatedAt = r.Payload["created_at"]
		item.UpdatedAt = r.Payload["updated_at"]
	}
	return item
}
