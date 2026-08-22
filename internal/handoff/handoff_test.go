package handoff

import (
	"github.com/zoro-cli/zoro.ai/internal/planner"
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
