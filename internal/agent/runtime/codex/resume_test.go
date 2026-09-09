package codex

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	"github.com/felinics/memoh/internal/agent/runtime/external"
	"github.com/felinics/memoh/internal/agent/runtime/toolmount"
	"github.com/felinics/memoh/internal/apperror"
	"github.com/felinics/memoh/internal/workspace/bridge"
	pb "github.com/felinics/memoh/internal/workspace/bridgepb"
	"github.com/felinics/memoh/internal/workspace/bridgesvc"
)

func TestMissingNativeThreadDoesNotStartFreshThread(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	pb.RegisterContainerServiceServer(server, bridgesvc.New(bridgesvc.Options{ReverseHTTP: bridgesvc.NewReverseHTTPBroker()}))
	go func() { _ = server.Serve(listener) }()
	defer server.Stop()
	connection, err := grpc.NewClient("passthrough:///test", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	if err != nil {
		t.Fatal(err)
	}
	client := bridge.NewClientFromConn(connection)
	defer func() { _ = client.Close() }()
	logger := slog.New(slog.DiscardHandler)
	local, remote := net.Pipe()
	defer func() { _ = remote.Close() }()
	rpc := newConn(local, nil, logger)
	defer func() { _ = rpc.Close() }()
	methods := make(chan string, 8)
	go func() {
		decoder := json.NewDecoder(remote)
		encoder := json.NewEncoder(remote)
		for {
			var request struct {
				ID     json.RawMessage `json:"id"`
				Method string          `json:"method"`
			}
			if decoder.Decode(&request) != nil {
				return
			}
			methods <- request.Method
			if encoder.Encode(map[string]any{"id": request.ID, "error": map[string]any{"code": -32600, "message": "native thread not found SECRET"}}) != nil {
				return
			}
		}
	}()
	srv := &appServer{conn: rpc, client: client, mountCtx: ctx, workspaceInfo: bridge.WorkspaceInfo{ACPToolsHTTPURL: "http://workspace.test/mcp"}, toolMounts: map[string]*toolmount.Mount{}, loadedThreads: map[string]bool{}}
	defer srv.stopToolMounts()
	d := &Driver{logger: logger}
	id, fresh, err := d.ensureThread(ctx, srv, Config{}, external.PromptInput{RuntimeMetadata: map[string]any{metadataThreadIDKey: "persisted-thread"}})
	if err == nil || id != "" || fresh {
		t.Fatalf("resume result: id=%q fresh=%v error=%v", id, fresh, err)
	}
	public, _ := apperror.PublicFrom(err, "test")
	publicJSON, marshalErr := json.Marshal(public)
	if marshalErr != nil || strings.Contains(string(publicJSON), "SECRET") {
		t.Fatalf("private resume failure leaked: %s (%v)", publicJSON, marshalErr)
	}
	if public.Code != apperror.CodeExternalRuntimeUnavailable {
		t.Fatalf("public error = %#v", public)
	}
	select {
	case method := <-methods:
		if method != "thread/resume" {
			t.Fatalf("first method=%q", method)
		}
	case <-ctx.Done():
		t.Fatal("thread/resume was never sent")
	}
	select {
	case method := <-methods:
		t.Fatalf("unexpected fallback RPC: %s", method)
	default:
	}
}
