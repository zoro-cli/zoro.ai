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
