// Package sparse provides a Go client for the sparse encoding Python service.
// The Python service loads the OpenSearch neural sparse model from HuggingFace
// and exposes HTTP endpoints for text → sparse vector encoding.
package sparse

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// SparseVector holds the non-zero components of a sparse text encoding.
type SparseVector struct {
	Indices []uint32  `json:"indices"`
	Values  []float32 `json:"values"`
}

// Client calls the Python sparse encoding service.
type Client struct {
	baseURL string
	http    *http.Client
}

// NewClient creates a sparse encoding client pointing to the Python service.
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		http:    &http.Client{Timeout: 60 * time.Second},
	}
}

// EncodeDocument encodes a document text into a sparse vector using the neural model.
func (c *Client) EncodeDocument(ctx context.Context, text string) (*SparseVector, error) {
	return c.encode(ctx, "/encode/document", text)
}

// EncodeQuery encodes a query text into a sparse vector (IDF-weighted tokenizer lookup).
func (c *Client) EncodeQuery(ctx context.Context, text string) (*SparseVector, error) {
	return c.encode(ctx, "/encode/query", text)
}

// EncodeDocuments encodes multiple document texts in a single batch call.
func (c *Client) EncodeDocuments(ctx context.Context, texts []string) ([]SparseVector, error) {
	body, err := json.Marshal(map[string]any{"texts": texts})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/encode/documents", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sparse encode failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("sparse encode error (status %d): %s", resp.StatusCode, string(respBody))
	}
	var vectors []SparseVector
	if err := json.NewDecoder(resp.Body).Decode(&vectors); err != nil {
		return nil, err
	}
	return vectors, nil
}

func (c *Client) encode(ctx context.Context, path, text string) (*SparseVector, error) {
	body, err := json.Marshal(map[string]string{"text": text})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sparse encode failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("sparse encode error (status %d): %s", resp.StatusCode, string(respBody))
	}
	var vec SparseVector
	if err := json.NewDecoder(resp.Body).Decode(&vec); err != nil {
		return nil, err
	}
	return &vec, nil
}
