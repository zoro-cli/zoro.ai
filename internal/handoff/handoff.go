package handoff

import (
	"bytes"
	"fmt"
	"github.com/zoro-cli/zoro.ai/internal/planner"
	"github.com/zoro-cli/zoro.ai/internal/repository"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

var States = []string{"ready", "implementing", "review", "done", "failed"}

type Metadata struct {
	Repository      string    `yaml:"repository"`
	ProjectItemID   string    `yaml:"project_item_id"`
	Issue           int       `yaml:"issue"`
	Title           string    `yaml:"title"`
	CreatedAt       time.Time `yaml:"created_at"`
	DirtyAtPlanning bool      `yaml:"dirty_at_planning"`
}

func Filename(issue int, title string) string {
	return fmt.Sprintf("%d-%s.md", issue, repository.Slug(title))
}
func Ensure(root, base string) error {
	for _, s := range States {
		if e := os.MkdirAll(filepath.Join(root, base, s), 0755); e != nil {
			return e
		}
	}
	return nil
}
func Render(m Metadata, p planner.Plan) ([]byte, error) {
	y, e := yaml.Marshal(m)
	if e != nil {
		return nil, e
	}
	var b bytes.Buffer
	b.WriteString("---\n")
	b.Write(y)
	b.WriteString("---\n\n# " + m.Title + "\n\n## Summary\n\n" + p.Summary + "\n\n## Objective\n\n" + p.Objective + "\n")
	section(&b, "Assumptions", p.Assumptions, false)
	section(&b, "Preparation", p.Preparation, false)
	section(&b, "Implementation steps", p.ImplementationSteps, true)
	section(&b, "Validation steps", p.ValidationSteps, true)
	section(&b, "Risks", p.Risks, false)
	b.WriteString("\n## Relevant files\n")
	for _, x := range p.RelevantFiles {
		b.WriteString("\n- `" + x.Path + "`: " + x.Reason)
		if x.ExpectedChange != nil {
			b.WriteString(" — " + *x.ExpectedChange)
		}
	}
	b.WriteString("\n\n## Proposed changes\n")
	for _, x := range p.ProposedChanges {
		b.WriteString("\n- ")
		if x.File != nil {
			b.WriteString("`" + *x.File + "`: ")
		}
		b.WriteString(x.Description)
		if x.Risk != nil {
			b.WriteString(" (Risk: " + *x.Risk + ")")
		}
	}
	b.WriteString("\n\n## Acceptance criteria\n")
	for _, x := range p.AcceptanceCriteria {
		b.WriteString("\n- [ ] " + x.Criterion)
		if x.Validation != nil {
			b.WriteString(" — " + *x.Validation)
		}
	}
	b.WriteByte('\n')
	return b.Bytes(), nil
}
func section(b *bytes.Buffer, name string, items []string, numbered bool) {
	b.WriteString("\n## " + name + "\n")
	for i, x := range items {
		if numbered {
			b.WriteString(fmt.Sprintf("\n%d. %s", i+1, x))
		} else {
			b.WriteString("\n- " + x)
		}
	}
	b.WriteByte('\n')
}
func Save(root, base string, m Metadata, p planner.Plan) (string, error) {
	if e := Ensure(root, base); e != nil {
		return "", e
	}
	exists, _, e := Find(root, base, m.ProjectItemID, m.Issue)
	if e != nil {
		return "", e
	}
	if exists != "" {
		return "", fmt.Errorf("handoff already exists: %s", exists)
	}
	b, e := Render(m, p)
	if e != nil {
		return "", e
	}
	path := filepath.Join(root, base, "ready", Filename(m.Issue, m.Title))
	return path, os.WriteFile(path, b, 0644)
}
func Parse(path string) (Metadata, error) {
	var m Metadata
	b, e := os.ReadFile(path)
	if e != nil {
		return m, e
	}
	parts := bytes.SplitN(b, []byte("---"), 3)
	if len(parts) < 3 {
		return m, fmt.Errorf("invalid handoff frontmatter")
	}
	e = yaml.Unmarshal(parts[1], &m)
	return m, e
}
func Find(root, base, itemID string, issue int) (string, Metadata, error) {
	for _, s := range States {
		files, e := filepath.Glob(filepath.Join(root, base, s, "*.md"))
		if e != nil {
			return "", Metadata{}, e
		}
		sort.Strings(files)
		for _, f := range files {
			m, e := Parse(f)
			if e != nil {
				continue
			}
			if itemID != "" && m.ProjectItemID == itemID || issue > 0 && m.Issue == issue {
				return f, m, nil
			}
		}
	}
	return "", Metadata{}, nil
}
func List(root, base, state string) ([]string, error) {
	f, e := filepath.Glob(filepath.Join(root, base, state, "*.md"))
	sort.Strings(f)
	return f, e
}
func Move(path, root, base, state string) (string, error) {
	dst := filepath.Join(root, base, state, filepath.Base(path))
	if _, e := os.Stat(dst); e == nil {
		return "", fmt.Errorf("destination exists: %s", dst)
	}
	return dst, os.Rename(path, dst)
}
func IssueFromArg(s string) (int, error) { return strconv.Atoi(strings.TrimPrefix(s, "#")) }
