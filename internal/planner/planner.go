package planner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/zoro-cli/zoro.ai/internal/app"
	gh "github.com/zoro-cli/zoro.ai/internal/github"
	"github.com/zoro-cli/zoro.ai/internal/repository"
	"io"
	"net/http"
	"strings"
	"time"
)

type RelevantFile struct {
	Path, Reason   string
	ExpectedChange *string `json:"expected_change,omitempty"`
}
type ProposedChange struct {
	File        *string `json:"file,omitempty"`
	Description string
	Risk        *string `json:"risk,omitempty"`
}
type AcceptanceCriterion struct {
	Criterion  string
	Validation *string `json:"validation,omitempty"`
}
type Plan struct {
	Summary, Objective  string
	Assumptions         []string              `json:"assumptions"`
	RelevantFiles       []RelevantFile        `json:"relevant_files"`
	ProposedChanges     []ProposedChange      `json:"proposed_changes"`
	Preparation         []string              `json:"preparation"`
	ImplementationSteps []string              `json:"implementation_steps"`
	ValidationSteps     []string              `json:"validation_steps"`
	Risks               []string              `json:"risks"`
	AcceptanceCriteria  []AcceptanceCriterion `json:"acceptance_criteria"`
}

func (p Plan) Validate() error {
	if strings.TrimSpace(p.Summary) == "" || strings.TrimSpace(p.Objective) == "" || len(p.ImplementationSteps) == 0 || len(p.AcceptanceCriteria) == 0 {
		return fmt.Errorf("%w: model plan is missing required fields", app.ErrPlanner)
	}
	return nil
}

type Request struct {
	Item       gh.ProjectItem
	Repository repository.Context
}
type Planner interface {
	Plan(context.Context, Request) (Plan, error)
}
type OpenAI struct {
	APIKey, Model, URL string
	HTTPClient         *http.Client
}

func New(key, model string) *OpenAI {
	return &OpenAI{key, model, "https://api.openai.com/v1/responses", &http.Client{Timeout: 3 * time.Minute}}
}
func (o *OpenAI) Plan(ctx context.Context, r Request) (Plan, error) {
	if o.APIKey == "" {
		return Plan{}, fmt.Errorf("%w: OPENAI_API_KEY is not set", app.ErrPlanner)
	}
	input, _ := json.Marshal(r)
	schema := map[string]any{"type": "object", "additionalProperties": false, "required": []string{"summary", "objective", "assumptions", "relevant_files", "proposed_changes", "preparation", "implementation_steps", "validation_steps", "risks", "acceptance_criteria"}, "properties": map[string]any{"summary": map[string]any{"type": "string"}, "objective": map[string]any{"type": "string"}, "assumptions": arrString(), "relevant_files": arrObj(map[string]any{"path": str(), "reason": str(), "expected_change": nullableString()}, []string{"path", "reason", "expected_change"}), "proposed_changes": arrObj(map[string]any{"file": nullableString(), "description": str(), "risk": nullableString()}, []string{"file", "description", "risk"}), "preparation": arrString(), "implementation_steps": arrString(), "validation_steps": arrString(), "risks": arrString(), "acceptance_criteria": arrObj(map[string]any{"criterion": str(), "validation": nullableString()}, []string{"criterion", "validation"})}}
	body := map[string]any{"model": o.Model, "instructions": "You are a read-only software planning agent. Analyze only the supplied issue and repository context. Return a precise implementation plan; never request or expose secrets.", "input": string(input), "text": map[string]any{"format": map[string]any{"type": "json_schema", "name": "handoff_plan", "strict": true, "schema": schema}}}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, "POST", o.URL, bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+o.APIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, e := o.HTTPClient.Do(req)
	if e != nil {
		return Plan{}, fmt.Errorf("%w: %v", app.ErrPlanner, e)
	}
	defer resp.Body.Close()
	rb, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode/100 != 2 {
		return Plan{}, fmt.Errorf("%w: OpenAI HTTP %d: %s", app.ErrPlanner, resp.StatusCode, strings.TrimSpace(string(rb)))
	}
	var env struct {
		Output []struct{ Content []struct{ Type, Text string } }
	}
	if e = json.Unmarshal(rb, &env); e != nil {
		return Plan{}, e
	}
	var text string
	for _, x := range env.Output {
		for _, c := range x.Content {
			if c.Type == "output_text" {
				text = c.Text
			}
		}
	}
	var p Plan
	if e = json.Unmarshal([]byte(text), &p); e != nil {
		return p, fmt.Errorf("%w: malformed structured output: %v", app.ErrPlanner, e)
	}
	return p, p.Validate()
}
func str() map[string]any            { return map[string]any{"type": "string"} }
func nullableString() map[string]any { return map[string]any{"type": []string{"string", "null"}} }
func arrString() map[string]any      { return map[string]any{"type": "array", "items": str()} }
func arrObj(props map[string]any, req []string) map[string]any {
	return map[string]any{"type": "array", "items": map[string]any{"type": "object", "additionalProperties": false, "properties": props, "required": req}}
}
