package config

import "testing"

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
