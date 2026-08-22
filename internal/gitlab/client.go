package gitlab

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/zoro-cli/zoro.ai/internal/app"
	"github.com/zoro-cli/zoro.ai/internal/config"
	gh "github.com/zoro-cli/zoro.ai/internal/github"
)

type Client struct {
	HTTPClient     *http.Client
	Token, BaseURL string
}

func New(token, baseURL string) *Client {
	return &Client{HTTPClient: &http.Client{Timeout: 30 * time.Second}, Token: token, BaseURL: strings.TrimRight(baseURL, "/")}
}

func (c *Client) request(ctx context.Context, method, endpoint string, payload any, out any) (http.Header, error) {
	var body io.Reader
	if payload != nil {
		b, e := json.Marshal(payload)
		if e != nil {
			return nil, e
		}
		body = bytes.NewReader(b)
	}
	r, e := http.NewRequestWithContext(ctx, method, c.BaseURL+endpoint, body)
	if e != nil {
		return nil, e
	}
	r.Header.Set("PRIVATE-TOKEN", c.Token)
	r.Header.Set("User-Agent", "zoro.ai/"+gh.Version)
	if payload != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	resp, e := c.HTTPClient.Do(r)
	if e != nil {
		return nil, fmt.Errorf("%w: %v", app.ErrGitLab, e)
	}
	defer resp.Body.Close()
	b, e := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if e != nil {
		return nil, e
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: HTTP %d: %s", app.ErrGitLab, resp.StatusCode, safeMessage(b))
	}
	if out != nil && len(b) != 0 {
		if e = json.Unmarshal(b, out); e != nil {
			return nil, fmt.Errorf("%w: decode response: %v", app.ErrGitLab, e)
		}
	}
	return resp.Header, nil
}
func safeMessage(b []byte) string {
	var v struct {
		Message any    `json:"message"`
		Error   string `json:"error"`
	}
	if json.Unmarshal(b, &v) == nil {
		if v.Error != "" {
			return v.Error
		}
		if s, ok := v.Message.(string); ok {
			return s
		}
	}
	return http.StatusText(http.StatusBadRequest)
}
func projectPath(p string) string { return url.PathEscape(p) }

func (c *Client) VerifyRepository(ctx context.Context, project, _ string) error {
	var out any
	_, e := c.request(ctx, "GET", "/api/v4/projects/"+projectPath(project), nil, &out)
	return e
}

type boardList struct {
	ID    int `json:"id"`
	Label struct {
		Name string `json:"name"`
	} `json:"label"`
}

func (c *Client) Project(ctx context.Context, cfg config.GitLabConfig) (gh.Project, error) {
	var meta struct {
		ID   int    `json:"id"`
		Path string `json:"path_with_namespace"`
	}
	if _, e := c.request(ctx, "GET", "/api/v4/projects/"+projectPath(cfg.Project), nil, &meta); e != nil {
		return gh.Project{}, e
	}
	var board struct {
		ID    int         `json:"id"`
		Name  string      `json:"name"`
		Lists []boardList `json:"lists"`
	}
	if _, e := c.request(ctx, "GET", fmt.Sprintf("/api/v4/projects/%s/boards/%d", projectPath(cfg.Project), cfg.BoardID), nil, &board); e != nil {
		return gh.Project{}, e
	}
	p := gh.Project{ID: strconv.Itoa(board.ID), Title: board.Name, Repository: cfg.Project, StatusOptions: map[string]string{}}
	for _, l := range board.Lists {
		p.StatusOptions[l.Label.Name] = strconv.Itoa(l.ID)
	}
	for _, name := range []string{cfg.Statuses.Backlog, cfg.Statuses.Ready, cfg.Statuses.Implementing, cfg.Statuses.Review, cfg.Statuses.Done} {
		if p.StatusOptions[name] == "" {
			return gh.Project{}, fmt.Errorf("%w: required GitLab board list %q not found", app.ErrProject, name)
		}
	}
	position := 0
	for _, l := range board.Lists {
		page := 1
		for {
			var issues []struct {
				ID, IID            int
				Title, Description string
				Labels             []string
				RelativePosition   int `json:"relative_position"`
			}
			h, e := c.request(ctx, "GET", fmt.Sprintf("/api/v4/projects/%s/boards/%d/lists/%d/issues?per_page=100&page=%d", projectPath(cfg.Project), cfg.BoardID, l.ID, page), nil, &issues)
			if e != nil {
				return gh.Project{}, e
			}
			for _, x := range issues {
				pos := x.RelativePosition
				if pos == 0 {
					pos = position
				}
				p.Items = append(p.Items, gh.ProjectItem{ID: strconv.Itoa(x.ID), ContentID: strconv.Itoa(x.ID), IssueNumber: x.IID, Title: x.Title, Body: x.Description, Status: l.Label.Name, Repository: cfg.Project, Position: pos})
				position++
			}
			next := h.Get("X-Next-Page")
			if next == "" {
				break
			}
			page, _ = strconv.Atoi(next)
		}
	}
	sort.SliceStable(p.Items, func(i, j int) bool { return p.Items[i].Position < p.Items[j].Position })
	return p, nil
}
func (c *Client) UpdateStatus(ctx context.Context, p gh.Project, itemID, status string) error {
	list := p.StatusOptions[status]
	if list == "" {
		return fmt.Errorf("%w: unknown status %q", app.ErrProject, status)
	}
	_, e := c.request(ctx, "POST", fmt.Sprintf("/api/v4/projects/%s/boards/%s/lists/%s/issues/%s", projectPath(p.Repository), p.ID, list, itemID), nil, nil)
	return e
}

type IssueComment struct {
	Body string `json:"body"`
}

func (c *Client) ListIssueComments(ctx context.Context, project, _ string, issue int) ([]gh.IssueComment, error) {
	var all []gh.IssueComment
	for page := 1; ; page++ {
		var notes []IssueComment
		h, e := c.request(ctx, "GET", fmt.Sprintf("/api/v4/projects/%s/issues/%d/notes?per_page=100&page=%d", projectPath(project), issue, page), nil, &notes)
		if e != nil {
			return nil, e
		}
		for _, n := range notes {
			all = append(all, gh.IssueComment{Body: n.Body})
		}
		if h.Get("X-Next-Page") == "" {
			return all, nil
		}
	}
}
func (c *Client) CreateIssueComment(ctx context.Context, project, _ string, issue int, body string) error {
	_, e := c.request(ctx, "POST", fmt.Sprintf("/api/v4/projects/%s/issues/%d/notes", projectPath(project), issue), IssueComment{Body: body}, nil)
	return e
}
