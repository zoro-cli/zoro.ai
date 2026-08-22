package config

import (
	"gopkg.in/yaml.v3"
	"strings"
	"testing"
)

func TestValidate(t *testing.T) {
	c := Default("acme", "app", 1)
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	cases := []func(*Config){func(c *Config) { c.Scheduler.Interval = "wat" }, func(c *Config) { c.Scheduler.Interval = "0s" }, func(c *Config) { c.GitHub.ProjectNumber = 0 }, func(c *Config) { c.GitHub.Statuses.Ready = "" }, func(c *Config) { c.Planning.MaxFiles = 0 }, func(c *Config) { c.Planning.MaxContextBytes = 0 }}
	for i, mutate := range cases {
		c := Default("acme", "app", 1)
		mutate(&c)
		if c.Validate() == nil {
			t.Errorf("case %d unexpectedly valid", i)
		}
	}
}
func TestLegacyConfigDefaultsToGitHub(t *testing.T) {
	c := Default("acme", "app", 2)
	c.Provider = ""
	b, _ := yaml.Marshal(c)
	var got Config
	if e := yaml.Unmarshal(b, &got); e != nil || got.EffectiveProvider() != "github" || got.Validate() != nil {
		t.Fatalf("legacy config: %+v %v", got, e)
	}
}
func TestGitLabValidation(t *testing.T) {
	c := Default("", "", 1)
	c.Provider = "gitlab"
	c.GitHub = GitHubConfig{}
	c.GitLab = GitLabConfig{BaseURL: "https://gitlab.example.com", Project: "group/sub/app", BoardID: 3, Statuses: Statuses{Backlog: "Backlog", Ready: "Ready", Implementing: "Doing", Review: "Review", Done: "Done"}}
	if e := c.Validate(); e != nil {
		t.Fatal(e)
	}
	for _, mutate := range []func(*Config){func(c *Config) { c.Provider = "other" }, func(c *Config) { c.GitLab.BaseURL = "ftp://bad" }, func(c *Config) { c.GitLab.BoardID = 0 }} {
		x := c
		mutate(&x)
		if e := x.Validate(); e == nil || !strings.Contains(e.Error(), "config error") {
			t.Fatalf("expected config error, got %v", e)
		}
	}
}
func TestSaveLoad(t *testing.T) {
	root := t.TempDir()
	want := Default("acme", "app", 4)
	if err := Save(root, want); err != nil {
		t.Fatal(err)
	}
	got, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if got.GitHub.Repo != "app" || got.GitHub.ProjectNumber != 4 {
		t.Fatalf("unexpected config: %+v", got)
	}
}
