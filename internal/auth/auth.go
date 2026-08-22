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

func GitLabToken(ctx context.Context) (string, error) {
	for _, k := range []string{"ZORO_GITLAB_TOKEN", "GITLAB_TOKEN"} {
		if raw := os.Getenv(k); raw != "" {
			if raw != strings.TrimSpace(raw) {
				return "", fmt.Errorf("%w: %s contains surrounding whitespace", app.ErrAuth, k)
			}
			return raw, nil
		}
	}
	b, e := exec.CommandContext(ctx, "glab", "auth", "token").Output()
	if e == nil && strings.TrimSpace(string(b)) != "" {
		return strings.TrimSpace(string(b)), nil
	}
	return "", fmt.Errorf("%w: set ZORO_GITLAB_TOKEN or GITLAB_TOKEN, or run glab auth login", app.ErrAuth)
}
