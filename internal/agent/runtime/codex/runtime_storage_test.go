package codex

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/felinics/memoh/internal/workspace/bridge"
)

type runtimeStorageExecFunc func(context.Context, string, string, int32, []byte, bridge.ExecOptions) (*bridge.ExecResult, error)

func (f runtimeStorageExecFunc) ExecWithOptions(ctx context.Context, command, workdir string, timeout int32, stdin []byte, opts bridge.ExecOptions) (*bridge.ExecResult, error) {
	return f(ctx, command, workdir, timeout, stdin, opts)
}

func TestPrepareRuntimeStorageExecContract(t *testing.T) {
	t.Parallel()
	home := codexHome("agent'with-quote")
	client := runtimeStorageExecFunc(func(ctx context.Context, command, workdir string, timeout int32, stdin []byte, opts bridge.ExecOptions) (*bridge.ExecResult, error) {
		if command != "/opt/memoh/toolkit/bin/python3 -I - '/data/.codex/agents/agent'\"'\"'with-quote'" {
			t.Fatalf("unsafe command quoting: %q", command)
		}
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > 10*time.Second || timeout != 10 {
			t.Fatal("helper must have bounded client and workspace deadlines")
		}
		if workdir != "/" || !opts.CleanEnv || len(opts.Env) != 1 || opts.Env[0] != "PATH="+containerPath {
			t.Fatalf("unexpected helper execution environment: %q %#v", workdir, opts)
		}
		if !bytes.Equal(stdin, runtimeStorageScript) {
			t.Fatal("helper must execute embedded script over stdin")
		}
		return &bridge.ExecResult{Stdout: "isolated\n"}, nil
	})
	if err := prepareRuntimeStorage(context.Background(), client, home); err != nil {
		t.Fatal(err)
	}
}

func TestPrepareRuntimeStorageResults(t *testing.T) {
	t.Parallel()
	transportErr := errors.New("transport failed")
	for _, tt := range []struct {
		name   string
		result *bridge.ExecResult
		err    error
		want   string
	}{
		{name: "existing local", result: &bridge.ExecResult{Stdout: "local\n"}},
		{name: "missing result", want: "missing helper result"},
		{name: "invalid output", result: &bridge.ExecResult{}, want: "invalid helper result"},
		{name: "transport", err: transportErr, want: "transport failed"},
		{name: "legacy", result: &bridge.ExecResult{ExitCode: 2, Stderr: "native_home_requires_drain\n"}, want: "drain this Agent"},
		{name: "remote tmp", result: &bridge.ExecResult{ExitCode: 2, Stderr: "runtime_root_not_local\n"}, want: "runtime_root_not_local"},
		{name: "missing python", result: &bridge.ExecResult{ExitCode: 127, Stderr: "untrusted credential text"}, want: "helper exited with status 127"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := runtimeStorageExecFunc(func(context.Context, string, string, int32, []byte, bridge.ExecOptions) (*bridge.ExecResult, error) {
				return tt.result, tt.err
			})
			err := prepareRuntimeStorage(context.Background(), client, codexHome("agent"))
			if tt.want == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) || strings.Contains(err.Error(), "untrusted credential text") {
				t.Fatalf("error = %v, want sanitized error containing %q", err, tt.want)
			}
			if tt.name == "legacy" && !errors.Is(err, errLegacyRuntimeStorage) {
				t.Fatal("legacy layout error must remain identifiable")
			}
			if tt.err != nil && !errors.Is(err, tt.err) {
				t.Fatal("transport error must remain identifiable")
			}
		})
	}
}

func TestRuntimeStorageFilesystemSafety(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("helper targets Linux workspace filesystems")
	}
	_, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is required to exercise the workspace helper")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "python3", "-I", "runtime_storage_test.py", "-v").CombinedOutput()
	if err != nil {
		t.Fatalf("workspace filesystem tests: %v\n%s", err, output)
	}
	t.Logf("%s", output)
}
