// Package qdrant wraps the official github.com/qdrant/go-client SDK,
// providing a thin facade for sparse-vector memory operations.
package qdrant

import (
	"context"
	"fmt"
	"strings"

	pb "github.com/qdrant/go-client/qdrant"
)

const (
	sparseVectorName = "sparse"
)

// Client wraps the official Qdrant gRPC client with sparse-memory-specific helpers.
type Client struct {
	inner      *pb.Client
	collection string
}

// NewClient creates a Qdrant client connected via gRPC.
// host should be a bare hostname/IP; port is the gRPC port (default 6334).
func NewClient(host string, port int, apiKey, collection string) (*Client, error) {
	if host == "" {
		host = "localhost"
	}
	if port == 0 {
		port = 6334
	}
	if collection == "" {
		collection = "memory"
	}

	cfg := &pb.Config{
		Host: host,
		Port: port,
	}
	if apiKey != "" {
		cfg.APIKey = apiKey
	}

	inner, err := pb.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("qdrant: connect: %w", err)
	}
	return &Client{inner: inner, collection: collection}, nil
}

// Close closes the underlying gRPC connection.
func (c *Client) Close() error {
	return c.inner.Close()
}

// EnsureCollection creates the collection with a named sparse vector config if it does not exist.
func (c *Client) EnsureCollection(ctx context.Context) error {
	exists, err := c.inner.CollectionExists(ctx, c.collection)
	if err != nil {
		return fmt.Errorf("qdrant: check collection: %w", err)
	}
	if exists {
		return nil
	}
	err = c.inner.CreateCollection(ctx, &pb.CreateCollection{
		CollectionName: c.collection,
		SparseVectorsConfig: pb.NewSparseVectorsConfig(map[string]*pb.SparseVectorParams{
			sparseVectorName: {},
		}),
	})
	if err != nil {
		return fmt.Errorf("qdrant: create collection: %w", err)
	}
	return nil
}

// SparseVector holds the non-zero components of a sparse text encoding.
type SparseVector struct {
	Indices []uint32
	Values  []float32
}

// SearchResult is one result from a sparse search or scroll.
type SearchResult struct {
	ID      string
	Score   float64
	Payload map[string]string
}

// Upsert inserts or updates points with named sparse vectors.
func (c *Client) Upsert(ctx context.Context, id string, vec SparseVector, payload map[string]string) error {
	wait := true
	_, err := c.inner.Upsert(ctx, &pb.UpsertPoints{
		CollectionName: c.collection,
		Wait:           &wait,
		Points: []*pb.PointStruct{
			{
				Id: pb.NewID(id),
				Vectors: pb.NewVectorsMap(map[string]*pb.Vector{
					sparseVectorName: {
						Data:    vec.Values,
						Indices: &pb.SparseIndices{Data: vec.Indices},
					},
				}),
				Payload: stringPayloadToValueMap(payload),
			},
		},
	})
	if err != nil {
		return fmt.Errorf("qdrant: upsert: %w", err)
	}
	return nil
}

// Search performs a sparse-vector query against the collection, filtered by bot_id.
func (c *Client) Search(ctx context.Context, vec SparseVector, botID string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 10
	}
	scored, err := c.inner.Query(ctx, &pb.QueryPoints{
		CollectionName: c.collection,
		Query:          pb.NewQuerySparse(vec.Indices, vec.Values),
		Using:          strPtr(sparseVectorName),
		Filter:         botFilter(botID),
		Limit:          uint64Ptr(uint64(limit)),
		WithPayload:    pb.NewWithPayload(true),
	})
	if err != nil {
		return nil, fmt.Errorf("qdrant: search: %w", err)
	}
	return scoredPointsToResults(scored), nil
}

// GetByID fetches a single point by UUID.
func (c *Client) GetByID(ctx context.Context, id string) (*SearchResult, error) {
	points, err := c.inner.Get(ctx, &pb.GetPoints{
		CollectionName: c.collection,
		Ids:            []*pb.PointId{pb.NewID(id)},
		WithPayload:    pb.NewWithPayload(true),
	})
	if err != nil {
		return nil, fmt.Errorf("qdrant: get: %w", err)
	}
	if len(points) == 0 {
		return nil, nil
	}
	r := retrievedPointToResult(points[0])
	return &r, nil
}

// Scroll returns all points matching bot_id, up to limit.
func (c *Client) Scroll(ctx context.Context, botID string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 1000
	}
	l := uint32(limit)
	points, err := c.inner.Scroll(ctx, &pb.ScrollPoints{
		CollectionName: c.collection,
		Filter:         botFilter(botID),
		Limit:          &l,
		WithPayload:    pb.NewWithPayload(true),
	})
	if err != nil {
		return nil, fmt.Errorf("qdrant: scroll: %w", err)
	}
	results := make([]SearchResult, 0, len(points))
	for _, p := range points {
		results = append(results, retrievedPointToResult(p))
	}
	return results, nil
}

// Count returns the number of points matching bot_id.
func (c *Client) Count(ctx context.Context, botID string) (int, error) {
	exact := true
	n, err := c.inner.Count(ctx, &pb.CountPoints{
		CollectionName: c.collection,
		Filter:         botFilter(botID),
		Exact:          &exact,
	})
	if err != nil {
		return 0, fmt.Errorf("qdrant: count: %w", err)
	}
	return int(n), nil
}

// DeleteByIDs removes specific points by their UUID strings.
func (c *Client) DeleteByIDs(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	pointIDs := make([]*pb.PointId, 0, len(ids))
	for _, id := range ids {
		if strings.TrimSpace(id) != "" {
			pointIDs = append(pointIDs, pb.NewID(strings.TrimSpace(id)))
		}
	}
	wait := true
	_, err := c.inner.Delete(ctx, &pb.DeletePoints{
		CollectionName: c.collection,
		Wait:           &wait,
		Points: &pb.PointsSelector{
			PointsSelectorOneOf: &pb.PointsSelector_Points{
				Points: &pb.PointsIdsList{Ids: pointIDs},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("qdrant: delete by ids: %w", err)
	}
	return nil
}

// DeleteByBotID removes all points for a given bot_id.
func (c *Client) DeleteByBotID(ctx context.Context, botID string) error {
	wait := true
	_, err := c.inner.Delete(ctx, &pb.DeletePoints{
		CollectionName: c.collection,
		Wait:           &wait,
		Points: &pb.PointsSelector{
			PointsSelectorOneOf: &pb.PointsSelector_Filter{
				Filter: botFilter(botID),
			},
		},
	})
	if err != nil {
		return fmt.Errorf("qdrant: delete by bot_id: %w", err)
	}
	return nil
}

// --- helpers ---

func botFilter(botID string) *pb.Filter {
	return &pb.Filter{
		Must: []*pb.Condition{
			pb.NewMatch("bot_id", botID),
		},
	}
}

func stringPayloadToValueMap(payload map[string]string) map[string]*pb.Value {
	m := make(map[string]*pb.Value, len(payload))
	for k, v := range payload {
		m[k] = pb.NewValueString(v)
	}
	return m
}

func valueMapToStringPayload(m map[string]*pb.Value) map[string]string {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		if v != nil {
			if sv := v.GetStringValue(); sv != "" {
				out[k] = sv
			}
		}
	}
	return out
}

func scoredPointsToResults(scored []*pb.ScoredPoint) []SearchResult {
	results := make([]SearchResult, 0, len(scored))
	for _, p := range scored {
		results = append(results, SearchResult{
			ID:      extractID(p.GetId()),
			Score:   float64(p.GetScore()),
			Payload: valueMapToStringPayload(p.GetPayload()),
		})
	}
	return results
}

func retrievedPointToResult(p *pb.RetrievedPoint) SearchResult {
	return SearchResult{
		ID:      extractID(p.GetId()),
		Payload: valueMapToStringPayload(p.GetPayload()),
	}
}

func extractID(id *pb.PointId) string {
	if id == nil {
		return ""
	}
	if uuid := id.GetUuid(); uuid != "" {
		return uuid
	}
	return fmt.Sprintf("%d", id.GetNum())
}

func strPtr(s string) *string { return &s }

func uint64Ptr(v uint64) *uint64 { return &v }
