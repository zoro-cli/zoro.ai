package auth

import (
	"context"
	"fmt"
	"github.com/zoro-cli/zoro.ai/internal/app"
	"os"
	"os/exec"
	"strings"
)

func GitHubToken(ctx context.Context) (string, error) {
	for _, k := range []string{"ZORO_GITHUB_TOKEN", "GH_TOKEN"} {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v, nil
		}
	}
	b, e := exec.CommandContext(ctx, "gh", "auth", "token").Output()
	if e == nil && strings.TrimSpace(string(b)) != "" {
		return strings.TrimSpace(string(b)), nil
	}
	return "", fmt.Errorf("%w: set ZORO_GITHUB_TOKEN or GH_TOKEN, or run gh auth login", app.ErrAuth)
}
