package codex

import (
	"context"
	"fmt"
	"github.com/zoro-cli/zoro.ai/internal/app"
	"github.com/zoro-cli/zoro.ai/internal/process"
	"os/exec"
)

func Available() bool { _, e := exec.LookPath("codex"); return e == nil }
func Run(ctx context.Context, root, handoff string) (process.Result, error) {
	if !Available() {
		return process.Result{}, fmt.Errorf("%w: Codex CLI was not found in PATH", app.ErrCodex)
	}
	prompt := "Follow repository instructions. Implement only the handoff at " + handoff + ". Inspect existing code before editing. Do not refactor unrelated code. Preserve user changes. Run appropriate tests and report affected files and validation."
	r, e := process.Run(ctx, root, "codex", "exec", "--full-auto", prompt)
	if e != nil {
		return r, fmt.Errorf("%w: Codex exited with code %d: %s", app.ErrCodex, r.ExitCode, r.Stderr)
	}
	return r, nil
}
