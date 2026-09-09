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

//go:embed runtime_tmp.py
var runtimeTmpScript []byte

var errLegacyRuntimeTmp = errors.New("codex runtime tmp is on an unsupported filesystem; drain this Agent's processes and migrate its tmp directory before reconnecting")

type runtimeTmpExecutor interface {
	ExecWithOptions(context.Context, string, string, int32, []byte, bridge.ExecOptions) (*bridge.ExecResult, error)
}

// prepareRuntimeTmp runs in the workspace, not on the API server's filesystem.
// The helper uses descriptor-relative, no-follow operations and never removes
// or replaces an existing entry. Credentials and native sessions stay in home.
func prepareRuntimeTmp(ctx context.Context, client runtimeTmpExecutor, home string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	command := "/opt/memoh/toolkit/bin/python3 -I - " + shellQuote(home)
	result, err := client.ExecWithOptions(ctx, command, "/", 10, runtimeTmpScript, bridge.ExecOptions{
		CleanEnv: true,
		Env:      []string{"PATH=" + containerPath},
	})
	if err != nil {
		return fmt.Errorf("prepare codex runtime tmp: %w", err)
	}
	if result == nil {
		return errors.New("prepare codex runtime tmp: missing helper result")
	}
	if result.ExitCode != 0 {
		if strings.TrimSpace(result.Stderr) == "legacy_tmp_requires_drain" {
			return errLegacyRuntimeTmp
		}
		// Report only known helper codes, not arbitrary remote stderr (which
		// could include sensitive data from an unexpected launcher).
		switch code := strings.TrimSpace(result.Stderr); code {
		case "runtime_root_not_local", "unknown_filesystem", "unsafe_path", "unsafe_local_permissions", "unexpected_tmp_link", "unsafe_tmp_entry", "tmp_publication_failed":
			return fmt.Errorf("prepare codex runtime tmp: %s", code)
		default:
			return fmt.Errorf("prepare codex runtime tmp: helper exited with status %d", result.ExitCode)
		}
	}
	switch strings.TrimSpace(result.Stdout) {
	case "local", "isolated":
		return nil
	default:
		return errors.New("prepare codex runtime tmp: invalid helper result")
	}
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
