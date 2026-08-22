package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/zoro-cli/zoro.ai/internal/app"
	"gopkg.in/yaml.v3"
)

const Path = ".zoro/config.yaml"

type Config struct {
	Version        int                  `yaml:"version"`
	Provider       string               `yaml:"provider,omitempty"`
	GitHub         GitHubConfig         `yaml:"github"`
	GitLab         GitLabConfig         `yaml:"gitlab,omitempty"`
	Scheduler      SchedulerConfig      `yaml:"scheduler"`
	Planning       PlanningConfig       `yaml:"planning"`
	Automation     AutomationConfig     `yaml:"automation"`
	Implementation ImplementationConfig `yaml:"implementation"`
	Handoff        HandoffConfig        `yaml:"handoff"`
	Behavior       BehaviorConfig       `yaml:"behavior"`
}
type GitLabConfig struct {
	BaseURL  string   `yaml:"base_url"`
	Project  string   `yaml:"project"`
	BoardID  int      `yaml:"board_id"`
	Statuses Statuses `yaml:"statuses"`
}
type GitHubConfig struct {
	Owner         string   `yaml:"owner"`
	Repo          string   `yaml:"repo"`
	ProjectNumber int      `yaml:"project_number"`
	StatusField   string   `yaml:"status_field"`
	Statuses      Statuses `yaml:"statuses"`
}
type Statuses struct {
	Backlog      string `yaml:"backlog"`
	Ready        string `yaml:"ready"`
	Implementing string `yaml:"implementing"`
	Review       string `yaml:"review"`
	Done         string `yaml:"done"`
}
type SchedulerConfig struct {
	Enabled  bool   `yaml:"enabled"`
	Interval string `yaml:"interval"`
}
type PlanningConfig struct {
	Provider        string `yaml:"provider"`
	Model           string `yaml:"model"`
	MaxFiles        int    `yaml:"max_files"`
	MaxContextBytes int    `yaml:"max_context_bytes"`
}
type AutomationConfig struct {
	AutoPlan      bool `yaml:"auto_plan"`
	AutoImplement bool `yaml:"auto_implement"`
}
type ImplementationConfig struct {
	Provider   string           `yaml:"provider"`
	Branch     BranchConfig     `yaml:"branch"`
	Validation ValidationConfig `yaml:"validation"`
}
type BranchConfig struct {
	Enabled bool   `yaml:"enabled"`
	Prefix  string `yaml:"prefix"`
}
type ValidationConfig struct {
	Enabled  bool     `yaml:"enabled"`
	Commands []string `yaml:"commands"`
}
type HandoffConfig struct {
	Directory string `yaml:"directory"`
}
type BehaviorConfig struct {
	MaxConcurrentTasks int  `yaml:"max_concurrent_tasks"`
	MoveToInProgress   bool `yaml:"move_to_in_progress_on_implement"`
	MoveToReview       bool `yaml:"move_to_review_on_success"`
}

func Default(owner, repo string, project int) Config {
	return Config{Version: 1, Provider: "github", GitHub: GitHubConfig{Owner: owner, Repo: repo, ProjectNumber: project, StatusField: "Status", Statuses: Statuses{Backlog: "Backlog", Ready: "Ready", Implementing: "In progress", Review: "In review", Done: "Done"}}, Scheduler: SchedulerConfig{Enabled: true, Interval: "1m"}, Planning: PlanningConfig{Provider: "openai", Model: "gpt-5.6", MaxFiles: 30, MaxContextBytes: 300000}, Automation: AutomationConfig{AutoPlan: true}, Implementation: ImplementationConfig{Provider: "codex", Branch: BranchConfig{Enabled: true, Prefix: "zoro"}, Validation: ValidationConfig{Enabled: true, Commands: []string{"go test ./...", "go vet ./..."}}}, Handoff: HandoffConfig{Directory: "handoff"}, Behavior: BehaviorConfig{MaxConcurrentTasks: 1, MoveToInProgress: true, MoveToReview: true}}
}
func (c Config) EffectiveProvider() string {
	if c.Provider == "" {
		return "github"
	}
	return c.Provider
}
func (c Config) Statuses() Statuses {
	if c.EffectiveProvider() == "gitlab" {
		return c.GitLab.Statuses
	}
	return c.GitHub.Statuses
}
func (c Config) Repository() string {
	if c.EffectiveProvider() == "gitlab" {
		return c.GitLab.Project
	}
	return c.GitHub.Owner + "/" + c.GitHub.Repo
}
func Load(root string) (Config, error) {
	var c Config
	b, e := os.ReadFile(filepath.Join(root, Path))
	if e != nil {
		return c, fmt.Errorf("%w: read %s: %v", app.ErrConfig, Path, e)
	}
	if e = yaml.Unmarshal(b, &c); e != nil {
		return c, fmt.Errorf("%w: decode: %v", app.ErrConfig, e)
	}
	return c, c.Validate()
}
func (c Config) Interval() (time.Duration, error) {
	d, e := time.ParseDuration(c.Scheduler.Interval)
	if e != nil || d <= 0 {
		return 0, fmt.Errorf("%w: scheduler.interval must be a positive duration", app.ErrConfig)
	}
	return d, nil
}
func (c Config) Validate() error {
	if c.Version != 1 {
		return fmt.Errorf("%w: version must be 1", app.ErrConfig)
	}
	provider := c.EffectiveProvider()
	if provider != "github" && provider != "gitlab" {
		return fmt.Errorf("%w: provider must be github or gitlab", app.ErrConfig)
	}
	if provider == "github" && (c.GitHub.Owner == "" || c.GitHub.Repo == "" || c.GitHub.ProjectNumber <= 0) {
		return fmt.Errorf("%w: github owner, repo, and positive project_number are required", app.ErrConfig)
	}
	if provider == "github" && (c.GitHub.StatusField == "" || !validStatuses(c.GitHub.Statuses)) {
		return fmt.Errorf("%w: status field and all status mappings are required", app.ErrConfig)
	}
	if provider == "gitlab" {
		u, e := url.Parse(c.GitLab.BaseURL)
		if e != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return fmt.Errorf("%w: gitlab.base_url must be an HTTP(S) URL", app.ErrConfig)
		}
		if c.GitLab.Project == "" || c.GitLab.BoardID <= 0 || !validStatuses(c.GitLab.Statuses) {
			return fmt.Errorf("%w: gitlab project, positive board_id, and all status mappings are required", app.ErrConfig)
		}
	}
	if _, e := c.Interval(); e != nil {
		return e
	}
	if c.Planning.Model == "" || c.Planning.MaxFiles <= 0 || c.Planning.MaxContextBytes <= 0 {
		return fmt.Errorf("%w: valid planning model and limits are required", app.ErrConfig)
	}
	if c.Handoff.Directory == "" || c.Behavior.MaxConcurrentTasks <= 0 {
		return fmt.Errorf("%w: handoff.directory and positive max_concurrent_tasks are required", app.ErrConfig)
	}
	return nil
}
func validStatuses(s Statuses) bool {
	return s.Backlog != "" && s.Ready != "" && s.Implementing != "" && s.Review != "" && s.Done != ""
}
func Save(root string, c Config) error {
	if e := c.Validate(); e != nil {
		return e
	}
	if e := os.MkdirAll(filepath.Join(root, ".zoro", "runtime"), 0755); e != nil {
		return e
	}
	b, e := yaml.Marshal(c)
	if e != nil {
		return e
	}
	return os.WriteFile(filepath.Join(root, Path), b, 0644)
}
