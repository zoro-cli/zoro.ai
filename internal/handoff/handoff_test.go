package handoff

import (
	"github.com/zoro-cli/zoro.ai/internal/planner"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

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
