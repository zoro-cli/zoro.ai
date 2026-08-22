package repository

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseRemote(t *testing.T) {
	for _, in := range []string{"https://github.com/acme/app.git", "git@github.com:acme/app.git", "ssh://git@github.com/acme/app.git"} {
		o, r, e := ParseRemote(in)
		if e != nil || o != "acme" || r != "app" {
			t.Fatalf("%q => %q %q %v", in, o, r, e)
		}
	}
}
func TestBranchName(t *testing.T) {
	if got := BranchName("zoro", 142, "Add refresh token rotation"); got != "zoro/142-add-refresh-token-rotation" {
		t.Fatal(got)
	}
}
func TestExcluded(t *testing.T) {
	for _, p := range []string{".env", "config/.env.local", "id_rsa", "cert.pem", "vendor/x.go", "node_modules/a.js", "credentials.json"} {
		if !excluded(p) {
			t.Errorf("should exclude %s", p)
		}
	}
	if excluded("internal/auth/auth.go") {
		t.Fatal("excluded normal source")
	}
}

func TestStatusExceptAllowsOnlySelectedHandoff(t *testing.T) {
	root := t.TempDir()
	cmd := exec.Command("git", "init", "-q")
	cmd.Dir = root
	if out, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("git init: %v: %s", e, out)
	}
	handoff := filepath.Join(root, "handoff", "ready", "1-work.md")
	if e := os.MkdirAll(filepath.Dir(handoff), 0755); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(handoff, []byte("handoff"), 0644); e != nil {
		t.Fatal(e)
	}
	if status, dirty, e := StatusExcept(context.Background(), root, handoff); e != nil || dirty {
		t.Fatalf("status=%q dirty=%t error=%v", status, dirty, e)
	}
	if e := os.WriteFile(filepath.Join(root, "unrelated.txt"), []byte("change"), 0644); e != nil {
		t.Fatal(e)
	}
	status, dirty, e := StatusExcept(context.Background(), root, handoff)
	if e != nil || !dirty || !strings.Contains(status, "unrelated.txt") {
		t.Fatalf("status=%q dirty=%t error=%v", status, dirty, e)
	}
}

func TestAddPathsAndCommitScopesHandoffMove(t *testing.T) {
	root := initRepository(t)
	ready := filepath.Join(root, "handoff", "ready", "9-work.md")
	implementing := filepath.Join(root, "handoff", "implementing", "9-work.md")
	writeTestFile(t, ready, "handoff")
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "initial")
	if e := os.MkdirAll(filepath.Dir(implementing), 0755); e != nil {
		t.Fatal(e)
	}
	if e := os.Rename(ready, implementing); e != nil {
		t.Fatal(e)
	}
	writeTestFile(t, filepath.Join(root, "unrelated.txt"), "leave me unstaged")
	if e := AddPaths(context.Background(), root, ready, implementing); e != nil {
		t.Fatal(e)
	}
	if e := Commit(context.Background(), root, "start issue #9"); e != nil {
		t.Fatal(e)
	}
	show := runGit(t, root, "show", "--format=", "--name-status", "HEAD")
	if !strings.Contains(show, "handoff/implementing/9-work.md") || strings.Contains(show, "unrelated.txt") {
		t.Fatalf("unexpected committed paths:\n%s", show)
	}
}

func TestAddAllHasStagedChangesAndCommit(t *testing.T) {
	root := initRepository(t)
	writeTestFile(t, filepath.Join(root, "tracked.txt"), "before")
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "initial")
	writeTestFile(t, filepath.Join(root, "tracked.txt"), "after")
	writeTestFile(t, filepath.Join(root, "new.txt"), "new")
	if e := AddAll(context.Background(), root); e != nil {
		t.Fatal(e)
	}
	staged, e := HasStagedChanges(context.Background(), root)
	if e != nil || !staged {
		t.Fatalf("staged=%t error=%v", staged, e)
	}
	if e = Commit(context.Background(), root, "complete issue #9"); e != nil {
		t.Fatal(e)
	}
	if got := runGit(t, root, "status", "--porcelain"); got != "" {
		t.Fatalf("dirty repository: %s", got)
	}
	staged, e = HasStagedChanges(context.Background(), root)
	if e != nil || staged {
		t.Fatalf("staged=%t error=%v", staged, e)
	}
}

func initRepository(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	runGit(t, root, "init", "-q")
	runGit(t, root, "config", "user.name", "Zoro Test")
	runGit(t, root, "config", "user.email", "zoro@example.invalid")
	return root
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if e := os.MkdirAll(filepath.Dir(path), 0755); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(path, []byte(content), 0644); e != nil {
		t.Fatal(e)
	}
}

func runGit(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	out, e := cmd.CombinedOutput()
	if e != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), e, out)
	}
	return strings.TrimSpace(string(out))
}
