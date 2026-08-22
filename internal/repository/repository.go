package repository

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/zoro-cli/zoro.ai/internal/app"
)

type ContextFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Reason  string `json:"reason"`
}
type Context struct {
	Root             string        `json:"root"`
	GitStatus        string        `json:"git_status"`
	Dirty            bool          `json:"dirty"`
	Instructions     []ContextFile `json:"instructions"`
	RelevantFiles    []ContextFile `json:"relevant_files"`
	TreeSummary      []string      `json:"tree_summary"`
	TotalContextSize int           `json:"total_context_size"`
}

func git(ctx context.Context, root string, args ...string) (string, error) {
	c := exec.CommandContext(ctx, "git", args...)
	c.Dir = root
	var out, er bytes.Buffer
	c.Stdout = &out
	c.Stderr = &er
	if e := c.Run(); e != nil {
		return "", fmt.Errorf("%w: git %s: %s", app.ErrRepository, strings.Join(args, " "), strings.TrimSpace(er.String()))
	}
	return strings.TrimSpace(out.String()), nil
}
func Root(ctx context.Context, dir string) (string, error) {
	s, e := git(ctx, dir, "rev-parse", "--show-toplevel")
	if e != nil {
		return "", fmt.Errorf("%w: not inside a Git repository", app.ErrRepository)
	}
	return filepath.Clean(s), nil
}
func Status(ctx context.Context, root string) (string, bool, error) {
	s, e := git(ctx, root, "status", "--porcelain")
	return s, s != "", e
}
func StatusExcept(ctx context.Context, root string, allowedPath string) (string, bool, error) {
	s, e := git(ctx, root, "status", "--porcelain", "--untracked-files=all")
	if e != nil {
		return "", false, e
	}
	rel, e := filepath.Rel(root, allowedPath)
	if e != nil {
		return "", false, e
	}
	rel = filepath.ToSlash(rel)
	var remaining []string
	for _, line := range strings.Split(s, "\n") {
		if line == "" {
			continue
		}
		path := strings.TrimSpace(line[3:])
		if strings.Trim(path, `"`) != rel {
			remaining = append(remaining, line)
		}
	}
	s = strings.Join(remaining, "\n")
	return s, s != "", nil
}
func Remote(ctx context.Context, root string) (string, string, error) {
	s, e := git(ctx, root, "remote", "get-url", "origin")
	if e != nil {
		return "", "", e
	}
	return ParseRemote(s)
}

var remoteRE = regexp.MustCompile(`(?i)(?:github\.com[:/])([^/]+)/([^/]+?)(?:\.git)?$`)

func ParseRemote(s string) (string, string, error) {
	m := remoteRE.FindStringSubmatch(strings.TrimSpace(s))
	if len(m) != 3 {
		return "", "", fmt.Errorf("%w: unsupported GitHub remote %q", app.ErrRepository, s)
	}
	return m[1], strings.TrimSuffix(m[2], ".git"), nil
}
func Slug(s string) string {
	s = strings.ToLower(s)
	s = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(s, "-")
	return strings.Trim(strings.TrimSpace(s), "-")
}
func BranchName(prefix string, issue int, title string) string {
	slug := Slug(title)
	if len(slug) > 48 {
		slug = strings.TrimRight(slug[:48], "-")
	}
	return fmt.Sprintf("%s/%d-%s", strings.Trim(prefix, "/"), issue, slug)
}
func BranchExists(ctx context.Context, root, name string) bool {
	_, e := git(ctx, root, "show-ref", "--verify", "--quiet", "refs/heads/"+name)
	return e == nil
}
func CreateBranch(ctx context.Context, root, name string) error {
	if BranchExists(ctx, root, name) {
		return fmt.Errorf("%w: branch %q already exists", app.ErrRepository, name)
	}
	_, e := git(ctx, root, "switch", "-c", name)
	return e
}

var secretPatterns = []string{".env", ".npmrc", ".pypirc", ".netrc", "id_rsa", "id_ed25519"}

func excluded(path string) bool {
	p := strings.ToLower(filepath.ToSlash(path))
	for _, d := range []string{".git/", "node_modules/", "vendor/", ".venv/", "dist/", "build/", "coverage/", "bin/", "tmp/"} {
		if strings.Contains("/"+p, "/"+d) {
			return true
		}
	}
	b := strings.ToLower(filepath.Base(p))
	for _, x := range secretPatterns {
		if b == x || strings.HasPrefix(b, x+".") {
			return true
		}
	}
	for _, x := range []string{".pem", ".key", ".p12", ".pfx"} {
		if strings.HasSuffix(b, x) {
			return true
		}
	}
	return strings.HasPrefix(b, "credentials") || strings.HasPrefix(b, "secrets")
}
func Collect(ctx context.Context, root, query string, maxFiles, maxBytes int) (Context, error) {
	st, dirty, e := Status(ctx, root)
	if e != nil {
		return Context{}, e
	}
	listed, e := git(ctx, root, "ls-files")
	if e != nil {
		return Context{}, e
	}
	paths := strings.Fields(listed)
	sort.Strings(paths)
	keywords := terms(query)
	meta := map[string]bool{"AGENTS.md": true, "CLAUDE.md": true, "README.md": true, "go.mod": true, "go.sum": true, "pyproject.toml": true, "package.json": true, "Cargo.toml": true, "Dockerfile": true, "docker-compose.yml": true, "docker-compose.yaml": true}
	c := Context{Root: root, GitStatus: st, Dirty: dirty}
	for _, p := range paths {
		if excluded(p) {
			continue
		}
		c.TreeSummary = append(c.TreeSummary, p)
		base := filepath.Base(p)
		reason := ""
		instruction := false
		if meta[base] || strings.HasPrefix(filepath.ToSlash(p), ".github/") || strings.HasPrefix(filepath.ToSlash(p), "docs/") {
			reason = "repository metadata"
			instruction = true
		} else {
			lp := strings.ToLower(p)
			for _, k := range keywords {
				if strings.Contains(lp, k) {
					reason = "matches issue keyword " + k
					break
				}
			}
		}
		if reason == "" {
			continue
		}
		b, er := os.ReadFile(filepath.Join(root, p))
		if er != nil || bytes.IndexByte(b, 0) >= 0 || !utf8.Valid(b) {
			continue
		}
		if c.TotalContextSize+len(b) > maxBytes {
			continue
		}
		f := ContextFile{p, string(b), reason}
		if instruction {
			c.Instructions = append(c.Instructions, f)
		} else {
			c.RelevantFiles = append(c.RelevantFiles, f)
		}
		c.TotalContextSize += len(b)
		if len(c.Instructions)+len(c.RelevantFiles) >= maxFiles {
			break
		}
	}
	return c, nil
}
func terms(s string) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]{2,}`).FindAllString(strings.ToLower(s), -1) {
		if !seen[x] && x != "the" && x != "and" && x != "with" {
			seen[x] = true
			out = append(out, x)
		}
	}
	sort.Strings(out)
	return out
}
