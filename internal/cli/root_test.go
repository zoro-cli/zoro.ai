package cli

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zoro-cli/zoro.ai/internal/config"
	gh "github.com/zoro-cli/zoro.ai/internal/github"
	"github.com/zoro-cli/zoro.ai/internal/handoff"
	"github.com/zoro-cli/zoro.ai/internal/planner"
	"github.com/zoro-cli/zoro.ai/internal/process"
	"github.com/zoro-cli/zoro.ai/internal/validation"
)

func TestDecideCycle(t *testing.T) {
	tests := []struct {
		name  string
		match handoff.Match
		cfg   config.AutomationConfig
		want  cycleAction
	}{
		{"new plan and implement", handoff.Match{}, config.AutomationConfig{AutoPlan: true, AutoImplement: true}, cyclePlan},
		{"new plan only", handoff.Match{}, config.AutomationConfig{AutoPlan: true}, cyclePlan},
		{"disabled", handoff.Match{}, config.AutomationConfig{}, cycleSkip},
		{"existing ready implement", handoff.Match{Path: "ready.md", State: "ready"}, config.AutomationConfig{AutoImplement: true}, cycleImplement},
		{"existing ready remains", handoff.Match{Path: "ready.md", State: "ready"}, config.AutomationConfig{AutoPlan: true}, cycleSkip},
	}
	for _, state := range []string{"implementing", "review", "done", "failed"} {
		tests = append(tests, struct {
			name  string
			match handoff.Match
			cfg   config.AutomationConfig
			want  cycleAction
		}{"existing " + state + " skipped", handoff.Match{Path: state + ".md", State: state}, config.AutomationConfig{AutoPlan: true, AutoImplement: true}, cycleSkip})
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := decideCycle(tt.match, tt.cfg); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEnsureHandoffCommentCreatesOnce(t *testing.T) {
	root := t.TempDir()
	cfg := config.Default("acme", "app", 1)
	item := gh.ProjectItem{ID: "item-7", IssueNumber: 7, Title: "Work", Status: cfg.GitHub.Statuses.Ready, Repository: "acme/app"}
	path, e := handoff.Save(root, cfg.Handoff.Directory, handoff.Metadata{Repository: "acme/app", ProjectItemID: item.ID, Issue: 7, Title: item.Title}, planner.Plan{Summary: "Plan body"})
	if e != nil {
		t.Fatal(e)
	}
	posts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			if posts == 0 {
				fmt.Fprint(w, `[]`)
			} else {
				fmt.Fprintf(w, `[{"body":%q}]`, handoff.CommentMarker("acme/app", item.ID))
			}
			return
		}
		posts++
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, `{}`)
	}))
	defer srv.Close()
	client := gh.New("token")
	client.RESTURL = srv.URL
	s := state{root: root, cfg: cfg, client: client}
	if e = ensureHandoffComment(context.Background(), s, path, item); e != nil {
		t.Fatal(e)
	}
	if e = ensureHandoffComment(context.Background(), s, path, item); e != nil {
		t.Fatal(e)
	}
	if posts != 1 {
		t.Fatalf("created %d comments", posts)
	}
}

func TestEnsureHandoffCommentRejectsNonIssue(t *testing.T) {
	cfg := config.Default("acme", "app", 1)
	e := ensureHandoffComment(context.Background(), state{cfg: cfg}, "unused", gh.ProjectItem{ID: "draft", Title: "Draft"})
	if e == nil || !strings.Contains(e.Error(), "not backed") {
		t.Fatalf("unexpected error %v", e)
	}
}

func TestImplementLockedCreatesLifecycleCommits(t *testing.T) {
	root, ready := implementationRepository(t)
	originalCodex, originalValidation := runCodex, runValidation
	t.Cleanup(func() { runCodex, runValidation = originalCodex, originalValidation })
	runCodex = func(_ context.Context, gotRoot, current string) (process.Result, error) {
		if got := testGit(t, gotRoot, "status", "--porcelain"); got != "" {
			t.Fatalf("Codex started in dirty repository: %s", got)
		}
		if !strings.Contains(filepath.ToSlash(current), "/handoff/implementing/") {
			t.Fatalf("unexpected handoff path: %s", current)
		}
		if e := os.WriteFile(filepath.Join(gotRoot, "implementation.txt"), []byte("done"), 0644); e != nil {
			t.Fatal(e)
		}
		return process.Result{}, nil
	}
	runValidation = func(context.Context, string, []string) ([]validation.Result, error) { return nil, nil }
	cfg := testImplementationConfig()
	s := state{root: root, cfg: cfg, project: gh.Project{}}
	item := gh.ProjectItem{ID: "item-9", IssueNumber: 9, Title: "Work"}
	if e := implementLocked(context.Background(), s, ready, item); e != nil {
		t.Fatal(e)
	}
	log := strings.Split(testGit(t, root, "log", "-2", "--format=%s"), "\n")
	if len(log) != 2 || log[0] != implementationCompleteMessage(9) || log[1] != implementationStartMessage(9) {
		t.Fatalf("unexpected lifecycle commits: %q", log)
	}
	firstPaths := testGit(t, root, "show", "--format=", "--name-status", "HEAD~1")
	if !strings.Contains(firstPaths, "handoff/implementing/9-work.md") || strings.Contains(firstPaths, "implementation.txt") {
		t.Fatalf("unexpected start commit:\n%s", firstPaths)
	}
	if got := testGit(t, root, "status", "--porcelain"); got != "" {
		t.Fatalf("repository is dirty: %s", got)
	}
}

func TestImplementLockedDoesNotCompleteCommitAfterCodexFailure(t *testing.T) {
	root, ready := implementationRepository(t)
	originalCodex := runCodex
	t.Cleanup(func() { runCodex = originalCodex })
	runCodex = func(context.Context, string, string) (process.Result, error) {
		return process.Result{}, context.Canceled
	}
	cfg := testImplementationConfig()
	e := implementLocked(context.Background(), state{root: root, cfg: cfg}, ready, gh.ProjectItem{ID: "item-9"})
	if e == nil {
		t.Fatal("expected Codex failure")
	}
	if got := testGit(t, root, "log", "-1", "--format=%s"); got != implementationStartMessage(9) {
		t.Fatalf("unexpected last commit: %q", got)
	}
	if _, statErr := os.Stat(filepath.Join(root, "handoff", "failed", "9-work.md")); statErr != nil {
		t.Fatalf("handoff was not moved to failed: %v", statErr)
	}
}

func implementationRepository(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	testGit(t, root, "init", "-q")
	testGit(t, root, "config", "user.name", "Zoro Test")
	testGit(t, root, "config", "user.email", "zoro@example.invalid")
	if e := handoff.Ensure(root, "handoff"); e != nil {
		t.Fatal(e)
	}
	ready := filepath.Join(root, "handoff", "ready", "9-work.md")
	if e := os.MkdirAll(filepath.Dir(ready), 0755); e != nil {
		t.Fatal(e)
	}
	content := "---\nissue: 9\ntitle: Work\nproject_item_id: item-9\n---\n"
	if e := os.WriteFile(ready, []byte(content), 0644); e != nil {
		t.Fatal(e)
	}
	testGit(t, root, "add", ".")
	testGit(t, root, "commit", "-m", "initial")
	return root, ready
}

func testImplementationConfig() config.Config {
	return config.Config{
		Handoff:        config.HandoffConfig{Directory: "handoff"},
		Implementation: config.ImplementationConfig{Validation: config.ValidationConfig{Enabled: true}},
	}
}

func testGit(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	out, e := cmd.CombinedOutput()
	if e != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), e, out)
	}
	return strings.TrimSpace(string(out))
}
