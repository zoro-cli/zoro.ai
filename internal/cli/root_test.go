package cli

import (
	"testing"

	"github.com/zoro-cli/zoro.ai/internal/config"
	"github.com/zoro-cli/zoro.ai/internal/handoff"
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
