package codex

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/felinics/memoh/internal/workspace/bridge"
)

//go:embed runtime_storage.py
var runtimeStorageScript []byte

var errLegacyRuntimeStorage = errors.New("codex native storage is on an unsupported filesystem; drain this Agent's processes and migrate its native home before reconnecting")

type runtimeStorageExecutor interface {
	ExecWithOptions(context.Context, string, string, int32, []byte, bridge.ExecOptions) (*bridge.ExecResult, error)
}

// prepareRuntimeStorage runs in the workspace, not on the API server's filesystem.
// Existing network homes require an explicit offline migration. All native
// files, including credentials and SQLite, move together to a persistent volume.
func prepareRuntimeStorage(ctx context.Context, client runtimeStorageExecutor, home string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	command := "/opt/memoh/toolkit/bin/python3 -I - " + shellQuote(home)
	result, err := client.ExecWithOptions(ctx, command, "/", 10, runtimeStorageScript, bridge.ExecOptions{
		CleanEnv: true,
		Env:      []string{"PATH=" + containerPath},
	})
	if err != nil {
		return fmt.Errorf("prepare codex native storage: %w", err)
	}
	if result == nil {
		return errors.New("prepare codex native storage: missing helper result")
	}
	if result.ExitCode != 0 {
		if strings.TrimSpace(result.Stderr) == "native_home_requires_drain" {
			return errLegacyRuntimeStorage
		}
		// Report only known helper codes, not arbitrary remote stderr (which
		// could include sensitive data from an unexpected launcher).
		switch code := strings.TrimSpace(result.Stderr); code {
		case "runtime_root_not_local", "unknown_filesystem", "unsafe_path", "unsafe_local_permissions", "unexpected_tmp_link", "unsafe_tmp_entry", "tmp_publication_failed", "native_volume_required", "native_volume_not_persistent_local", "invalid_storage_identity", "native_volume_identity_mismatch", "native_state_missing", "orphaned_native_state", "unsafe_home_entry", "native_home_publication_failed", "legacy_tmp_requires_drain":
			return fmt.Errorf("prepare codex native storage: %s", code)
		default:
			return fmt.Errorf("prepare codex native storage: helper exited with status %d", result.ExitCode)
		}
	}
	switch strings.TrimSpace(result.Stdout) {
	case "local", "isolated":
		return nil
	default:
		return errors.New("prepare codex native storage: invalid helper result")
	}
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
