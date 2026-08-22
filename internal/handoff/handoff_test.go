package handoff

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/zoro-cli/zoro.ai/internal/planner"
)

func TestFilename(t *testing.T) {
	tests := []struct {
		name  string
		issue int
		title string
		want  string
	}{
		{name: "short unchanged", issue: 42, title: "Fix Login", want: "42-fix-login.md"},
		{name: "empty slug unchanged", issue: 7, title: "!!!", want: "7-.md"},
		{name: "exact boundary", issue: 42, title: strings.Repeat("a", 134), want: "42-" + strings.Repeat("a", 134) + ".md"},
		{name: "over boundary", issue: 42, title: strings.Repeat("a", 135), want: "42-" + strings.Repeat("a", 134) + ".md"},
		{name: "trailing separator removed", issue: 42, title: strings.Repeat("a", 133) + " long", want: "42-" + strings.Repeat("a", 133) + ".md"},
		{name: "issue length adjusts budget", issue: 123456, title: strings.Repeat("a", 140), want: "123456-" + strings.Repeat("a", 130) + ".md"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Filename(tt.issue, tt.title)
			if got != tt.want {
				t.Fatalf("Filename(%d, %q) = %q, want %q", tt.issue, tt.title, got, tt.want)
			}
			if utf8.RuneCountInString(got) > maxFilenameLength {
				t.Fatalf("filename length = %d, want <= %d", utf8.RuneCountInString(got), maxFilenameLength)
			}
			if Filename(tt.issue, tt.title) != got {
				t.Fatal("filename generation is not deterministic")
			}
		})
	}
}

func TestRenderAndParse(t *testing.T) {
	m := Metadata{Repository: "acme/app", ProjectItemID: "PVTI_1", Issue: 42, Title: "Fix Login", CreatedAt: time.Unix(0, 0).UTC()}
	p := planner.Plan{Summary: "Fix it", Objective: "Working login", ImplementationSteps: []string{"Edit auth"}, AcceptanceCriteria: []planner.AcceptanceCriterion{{Criterion: "Login works"}}}
	b, e := Render(m, p)
	if e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(string(b), "- [ ] Login works") {
		t.Fatal(string(b))
	}
	root := t.TempDir()
	path, e := Save(root, "handoff", m, p)
	if e != nil {
		t.Fatal(e)
	}
	got, e := Parse(path)
	if e != nil || got.ProjectItemID != "PVTI_1" {
		t.Fatalf("%+v %v", got, e)
	}
	if _, e := Save(root, "handoff", m, p); e == nil {
		t.Fatal("duplicate accepted")
	}
}

func TestFindMatchReportsLifecycleState(t *testing.T) {
	for _, state := range States {
		t.Run(state, func(t *testing.T) {
			root := t.TempDir()
			m := Metadata{ProjectItemID: "item", Issue: 7, Title: "Work"}
			path, e := Save(root, "handoff", m, planner.Plan{})
			if e != nil {
				t.Fatal(e)
			}
			if state != "ready" {
				path, e = Move(path, root, "handoff", state)
				if e != nil {
					t.Fatal(e)
				}
			}
			match, e := FindMatch(root, "handoff", "item", 7)
			if e != nil || match.State != state || match.Path != path || filepath.Base(match.Path) != Filename(7, "Work") {
				t.Fatalf("match=%+v error=%v", match, e)
			}
		})
	}
}
