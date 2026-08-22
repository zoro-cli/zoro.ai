package validation

import (
	"context"
	"fmt"
	"github.com/zoro-cli/zoro.ai/internal/app"
	"github.com/zoro-cli/zoro.ai/internal/process"
	"runtime"
)

type Result struct {
	Command string
	process.Result
}

func Run(ctx context.Context, root string, commands []string) ([]Result, error) {
	var out []Result
	for _, cmd := range commands {
		shell, args := "/bin/sh", []string{"-c", cmd}
		if runtime.GOOS == "windows" {
			shell, args = "cmd.exe", []string{"/C", cmd}
		}
		r, e := process.Run(ctx, root, shell, args...)
		out = append(out, Result{cmd, r})
		if e != nil {
			return out, fmt.Errorf("%w: %q exited with code %d", app.ErrValidation, cmd, r.ExitCode)
		}
	}
	return out, nil
}
