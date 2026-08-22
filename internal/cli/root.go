package cli

import (
	"context"
	"fmt"
	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"github.com/zoro-cli/zoro.ai/internal/auth"
	"github.com/zoro-cli/zoro.ai/internal/codex"
	"github.com/zoro-cli/zoro.ai/internal/config"
	gh "github.com/zoro-cli/zoro.ai/internal/github"
	"github.com/zoro-cli/zoro.ai/internal/handoff"
	"github.com/zoro-cli/zoro.ai/internal/planner"
	"github.com/zoro-cli/zoro.ai/internal/repository"
	zr "github.com/zoro-cli/zoro.ai/internal/runtime"
	"github.com/zoro-cli/zoro.ai/internal/validation"
	"gopkg.in/yaml.v3"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type state struct {
	root    string
	cfg     config.Config
	client  *gh.Client
	project gh.Project
}

func Execute() error {
	ctx, cancel := signalContext()
	defer cancel()
	r := rootCommand()
	r.SetContext(ctx)
	return r.Execute()
}
func rootCommand() *cobra.Command {
	r := &cobra.Command{Use: "zoro", Short: "Local-first agentic development orchestrator", SilenceUsage: true, Version: gh.Version}
	r.PersistentFlags().Bool("verbose", false, "show detailed progress")
	r.AddCommand(initCmd(), authCmd(), doctorCmd(), boardCmd(), readyCmd(), planCmd(), implementCmd(), runCmd(), statusCmd(), configCmd())
	return r
}
func rootOnly(ctx context.Context) (string, config.Config, error) {
	wd, e := os.Getwd()
	if e != nil {
		return "", config.Config{}, e
	}
	root, e := repository.Root(ctx, wd)
	if e != nil {
		return "", config.Config{}, e
	}
	cfg, e := config.Load(root)
	return root, cfg, e
}
func remoteState(ctx context.Context) (state, error) {
	root, cfg, e := rootOnly(ctx)
	if e != nil {
		return state{}, e
	}
	token, e := auth.GitHubToken(ctx)
	if e != nil {
		return state{}, e
	}
	cl := gh.New(token)
	p, e := cl.Project(ctx, cfg.GitHub)
	return state{root, cfg, cl, p}, e
}
func initCmd() *cobra.Command {
	var project int
	c := &cobra.Command{Use: "init", Short: "Initialize Zoro in the current Git repository", RunE: func(cmd *cobra.Command, args []string) error {
		wd, _ := os.Getwd()
		root, e := repository.Root(cmd.Context(), wd)
		if e != nil {
			return e
		}
		owner, repo, e := repository.Remote(cmd.Context(), root)
		if e != nil {
			return e
		}
		if project <= 0 {
			return fmt.Errorf("--project must be a positive GitHub Project number")
		}
		cfg := config.Default(owner, repo, project)
		if e = config.Save(root, cfg); e != nil {
			return e
		}
		if e = handoff.Ensure(root, cfg.Handoff.Directory); e != nil {
			return e
		}
		if e = ensureGitignore(root, ".zoro/runtime/"); e != nil {
			return e
		}
		fmt.Printf("Initialized zoro.ai for %s/%s (project %d)\n", owner, repo, project)
		return nil
	}}
	c.Flags().IntVar(&project, "project", 0, "GitHub Project number (required)")
	_ = c.MarkFlagRequired("project")
	return c
}
func ensureGitignore(root, line string) error {
	p := filepath.Join(root, ".gitignore")
	b, _ := os.ReadFile(p)
	for _, x := range strings.Split(string(b), "\n") {
		if strings.TrimSpace(x) == line {
			return nil
		}
	}
	prefix := ""
	if len(b) > 0 && !strings.HasSuffix(string(b), "\n") {
		prefix = "\n"
	}
	f, e := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if e != nil {
		return e
	}
	defer f.Close()
	_, e = f.WriteString(prefix + line + "\n")
	return e
}
func authCmd() *cobra.Command {
	return &cobra.Command{Use: "auth", Short: "Verify GitHub credentials and access", RunE: func(cmd *cobra.Command, args []string) error {
		s, e := remoteState(cmd.Context())
		if e != nil {
			return e
		}
		if e = s.client.VerifyRepository(cmd.Context(), s.cfg.GitHub.Owner, s.cfg.GitHub.Repo); e != nil {
			return e
		}
		fmt.Printf("Authenticated; repository and project %q are accessible.\n", s.project.Title)
		return nil
	}}
}
func boardCmd() *cobra.Command {
	return &cobra.Command{Use: "board", Short: "Show project status counts", RunE: func(cmd *cobra.Command, args []string) error {
		s, e := remoteState(cmd.Context())
		if e != nil {
			return e
		}
		labels := []string{s.cfg.GitHub.Statuses.Backlog, s.cfg.GitHub.Statuses.Ready, s.cfg.GitHub.Statuses.Implementing, s.cfg.GitHub.Statuses.Review, s.cfg.GitHub.Statuses.Done}
		for _, label := range labels {
			n := 0
			for _, it := range s.project.Items {
				if it.Status == label {
					n++
				}
			}
			fmt.Printf("%-14s %d\n", label, n)
		}
		return nil
	}}
}
func readyCmd() *cobra.Command {
	return &cobra.Command{Use: "ready", Short: "List ordered Ready items", RunE: func(cmd *cobra.Command, args []string) error {
		s, e := remoteState(cmd.Context())
		if e != nil {
			return e
		}
		fmt.Println("Ready")
		for i, it := range s.project.Ready(s.cfg.GitHub.Statuses.Ready) {
			fmt.Printf("%d. #%d %s\n", i+1, it.IssueNumber, it.Title)
		}
		return nil
	}}
}
func planCmd() *cobra.Command {
	return &cobra.Command{Use: "plan [issue]", Args: cobra.MaximumNArgs(1), Short: "Plan a Ready item", RunE: func(cmd *cobra.Command, args []string) error {
		s, e := remoteState(cmd.Context())
		if e != nil {
			return e
		}
		var it gh.ProjectItem
		if len(args) > 0 {
			n, e := strconv.Atoi(strings.TrimPrefix(args[0], "#"))
			if e != nil {
				return fmt.Errorf("invalid issue number")
			}
			for _, x := range s.project.Items {
				if x.IssueNumber == n {
					it = x
					break
				}
			}
		} else {
			r := s.project.Ready(s.cfg.GitHub.Statuses.Ready)
			if len(r) > 0 {
				it = r[0]
			}
		}
		if it.ID == "" {
			return fmt.Errorf("no matching project item")
		}
		path, e := createPlan(cmd.Context(), s, it)
		if e != nil {
			return e
		}
		fmt.Println(path)
		return nil
	}}
}
func createPlan(ctx context.Context, s state, it gh.ProjectItem) (string, error) {
	if p, _, _ := handoff.Find(s.root, s.cfg.Handoff.Directory, it.ID, it.IssueNumber); p != "" {
		return "", fmt.Errorf("item already has a handoff: %s", p)
	}
	rc, e := repository.Collect(ctx, s.root, it.Title+" "+it.Body, s.cfg.Planning.MaxFiles, s.cfg.Planning.MaxContextBytes)
	if e != nil {
		return "", e
	}
	p, e := planner.New(os.Getenv("OPENAI_API_KEY"), s.cfg.Planning.Model).Plan(ctx, planner.Request{Item: it, Repository: rc})
	if e != nil {
		return "", e
	}
	return handoff.Save(s.root, s.cfg.Handoff.Directory, handoff.Metadata{Repository: s.cfg.GitHub.Owner + "/" + s.cfg.GitHub.Repo, ProjectItemID: it.ID, Issue: it.IssueNumber, Title: it.Title, CreatedAt: time.Now().UTC(), DirtyAtPlanning: rc.Dirty}, p)
}

func implementCmd() *cobra.Command {
	return &cobra.Command{Use: "implement [issue]", Args: cobra.MaximumNArgs(1), Short: "Implement a ready handoff with Codex", RunE: func(cmd *cobra.Command, args []string) error {
		root, cfg, e := rootOnly(cmd.Context())
		if e != nil {
			return e
		}
		files, e := handoff.List(root, cfg.Handoff.Directory, "ready")
		if e != nil {
			return e
		}
		var selected string
		if len(args) > 0 {
			n, e := handoff.IssueFromArg(args[0])
			if e != nil {
				return fmt.Errorf("invalid issue number")
			}
			for _, f := range files {
				m, _ := handoff.Parse(f)
				if m.Issue == n {
					selected = f
					break
				}
			}
		} else if len(files) == 1 {
			selected = files[0]
		} else if len(files) > 1 {
			opts := make([]huh.Option[string], 0, len(files))
			for _, f := range files {
				m, _ := handoff.Parse(f)
				opts = append(opts, huh.NewOption(fmt.Sprintf("#%d %s", m.Issue, m.Title), f))
			}
			if e = huh.NewSelect[string]().Title("Select a handoff").Options(opts...).Value(&selected).Run(); e != nil {
				return e
			}
		}
		if selected == "" {
			return fmt.Errorf("no ready handoff found")
		}
		return implement(cmd.Context(), root, cfg, selected)
	}}
}
func implement(ctx context.Context, root string, cfg config.Config, path string) error {
	lock, e := zr.Acquire(root)
	if e != nil {
		return e
	}
	defer lock.Release()
	status, dirty, e := repository.Status(ctx, root)
	if e != nil {
		return e
	}
	if dirty {
		return fmt.Errorf("cannot start implementation; repository contains uncommitted changes:\n%s", status)
	}
	m, e := handoff.Parse(path)
	if e != nil {
		return e
	}
	s, e := remoteState(ctx)
	if e != nil {
		return e
	}
	var item gh.ProjectItem
	for _, x := range s.project.Items {
		if x.ID == m.ProjectItemID || x.IssueNumber == m.Issue {
			item = x
			break
		}
	}
	if item.ID == "" {
		return fmt.Errorf("project item for handoff was not found")
	}
	if cfg.Implementation.Branch.Enabled {
		branch := repository.BranchName(cfg.Implementation.Branch.Prefix, m.Issue, m.Title)
		if e = repository.CreateBranch(ctx, root, branch); e != nil {
			return e
		}
	}
	if cfg.Behavior.MoveToInProgress {
		if e = s.client.UpdateStatus(ctx, s.project, item.ID, cfg.GitHub.Statuses.Implementing); e != nil {
			return e
		}
	}
	current, e := handoff.Move(path, root, cfg.Handoff.Directory, "implementing")
	if e != nil {
		return e
	}
	result, e := codex.Run(ctx, root, current)
	if e != nil {
		_, _ = handoff.Move(current, root, cfg.Handoff.Directory, "failed")
		return e
	}
	fmt.Print(result.Stdout)
	if cfg.Implementation.Validation.Enabled {
		results, e := validation.Run(ctx, root, cfg.Implementation.Validation.Commands)
		for _, r := range results {
			fmt.Printf("Validated %s (%s)\n", r.Command, r.Duration.Round(time.Millisecond))
		}
		if e != nil {
			_, _ = handoff.Move(current, root, cfg.Handoff.Directory, "failed")
			return e
		}
	}
	review, e := handoff.Move(current, root, cfg.Handoff.Directory, "review")
	if e != nil {
		return e
	}
	if cfg.Behavior.MoveToReview {
		if e = s.client.UpdateStatus(ctx, s.project, item.ID, cfg.GitHub.Statuses.Review); e != nil {
			return fmt.Errorf("local implementation succeeded at %s, but board synchronization failed: %w", review, e)
		}
	}
	fmt.Printf("Implementation ready for review: %s\n", review)
	return nil
}
func runCmd() *cobra.Command {
	var once bool
	c := &cobra.Command{Use: "run", Short: "Poll and process Ready work", RunE: func(cmd *cobra.Command, args []string) error {
		root, cfg, e := rootOnly(cmd.Context())
		if e != nil {
			return e
		}
		cycle := func() error {
			lock, e := zr.Acquire(root)
			if e != nil {
				return e
			}
			s, e := remoteState(cmd.Context())
			if e != nil {
				lock.Release()
				return e
			}
			items := s.project.Ready(cfg.GitHub.Statuses.Ready)
			if len(items) == 0 {
				lock.Release()
				fmt.Println("No Ready items.")
				return nil
			}
			if !cfg.Automation.AutoPlan {
				lock.Release()
				fmt.Println("Automatic planning is disabled.")
				return nil
			}
			path, e := createPlan(cmd.Context(), s, items[0])
			lock.Release()
			if e != nil {
				if strings.Contains(e.Error(), "already has a handoff") {
					fmt.Println(e)
					return nil
				}
				return e
			}
			fmt.Printf("Handoff created: %s\n", path)
			if cfg.Automation.AutoImplement {
				return implement(cmd.Context(), root, cfg, path)
			}
			return nil
		}
		if once {
			return cycle()
		}
		d, e := cfg.Interval()
		if e != nil {
			return e
		}
		if e = cycle(); e != nil {
			fmt.Fprintln(os.Stderr, e)
		}
		ticker := time.NewTicker(d)
		defer ticker.Stop()
		for {
			select {
			case <-cmd.Context().Done():
				return nil
			case <-ticker.C:
				if e = cycle(); e != nil {
					fmt.Fprintln(os.Stderr, e)
				}
			}
		}
	}}
	c.Flags().BoolVar(&once, "once", false, "perform one polling cycle")
	return c
}
func statusCmd() *cobra.Command {
	return &cobra.Command{Use: "status", Short: "Show local orchestration state", RunE: func(cmd *cobra.Command, args []string) error {
		root, cfg, e := rootOnly(cmd.Context())
		if e != nil {
			return e
		}
		fmt.Printf("Repository          %s\nGitHub project      %s/%s #%d\nScheduler enabled   %t\nPolling interval    %s\nAuto implement      %t\n", root, cfg.GitHub.Owner, cfg.GitHub.Repo, cfg.GitHub.ProjectNumber, cfg.Scheduler.Enabled, cfg.Scheduler.Interval, cfg.Automation.AutoImplement)
		for _, state := range []string{"ready", "implementing", "review", "failed"} {
			f, _ := handoff.List(root, cfg.Handoff.Directory, state)
			fmt.Printf("%-18s %d\n", strings.Title(state)+" handoffs", len(f))
		}
		lock, e := zr.Acquire(root)
		if e != nil {
			fmt.Println("Lock state          active")
		} else {
			fmt.Println("Lock state          available")
			lock.Release()
		}
		return nil
	}}
}
func configCmd() *cobra.Command {
	c := &cobra.Command{Use: "config [path]", Args: cobra.MaximumNArgs(1), Short: "Print effective non-secret configuration", RunE: func(cmd *cobra.Command, args []string) error {
		root, cfg, e := rootOnly(cmd.Context())
		if e != nil {
			return e
		}
		if len(args) > 0 {
			if args[0] != "path" {
				return fmt.Errorf("only 'path' is supported")
			}
			fmt.Println(filepath.Join(root, config.Path))
			return nil
		}
		b, _ := yaml.Marshal(cfg)
		fmt.Print(string(b))
		return nil
	}}
	return c
}
func doctorCmd() *cobra.Command {
	return &cobra.Command{Use: "doctor", Short: "Diagnose the local environment", RunE: func(cmd *cobra.Command, args []string) error {
		failed := false
		check := func(name string, ok bool, warning bool) {
			mark := "✓"
			if !ok {
				mark = "✗"
				if !warning {
					failed = true
				}
			}
			fmt.Printf("%-22s %s\n", name, mark)
		}
		wd, _ := os.Getwd()
		root, e := repository.Root(cmd.Context(), wd)
		check("Git repository", e == nil, false)
		check("Git executable", look("git"), false)
		check("GitHub CLI", look("gh"), true)
		check("Codex CLI", codex.Available(), false)
		check("Go runtime", look("go"), true)
		check("OpenAI API key", os.Getenv("OPENAI_API_KEY") != "", false)
		if e == nil {
			cfg, ce := config.Load(root)
			check("Configuration", ce == nil, false)
			_, dirty, _ := repository.Status(cmd.Context(), root)
			check("Repository clean", !dirty, true)
			if ce == nil {
				dirs := true
				for _, s := range handoff.States {
					if st, se := os.Stat(filepath.Join(root, cfg.Handoff.Directory, s)); se != nil || !st.IsDir() {
						dirs = false
					}
				}
				check("Handoff directories", dirs, false)
				token, te := auth.GitHubToken(cmd.Context())
				check("GitHub auth", te == nil, false)
				if te == nil {
					cl := gh.New(token)
					check("GitHub repository", cl.VerifyRepository(cmd.Context(), cfg.GitHub.Owner, cfg.GitHub.Repo) == nil, false)
					_, pe := cl.Project(cmd.Context(), cfg.GitHub)
					check("GitHub project", pe == nil, false)
				}
			}
		}
		if failed {
			return fmt.Errorf("one or more required checks failed")
		}
		return nil
	}}
}
func look(name string) bool { _, e := exec.LookPath(name); return e == nil }
func signalContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}
