package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/zoro-cli/zoro.ai/internal/app"
	"github.com/zoro-cli/zoro.ai/internal/config"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const Version = "0.1.0"

type Client struct {
	HTTPClient                 *http.Client
	Token, GraphQLURL, RESTURL string
}
type ProjectItem struct {
	ID                              string
	ContentID                       string
	IssueNumber                     int
	Title, Body, Status, Repository string
	Position                        int
}
type Project struct {
	ID, Title, StatusFieldID string
	StatusOptions            map[string]string
	Items                    []ProjectItem
}

type IssueComment struct {
	Body string `json:"body"`
}

func New(token string) *Client {
	return &Client{&http.Client{Timeout: 30 * time.Second}, token, "https://api.github.com/graphql", "https://api.github.com"}
}
func (c *Client) request(ctx context.Context, method, url string, body []byte) ([]byte, error) {
	r, e := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if e != nil {
		return nil, e
	}
	r.Header.Set("Authorization", "Bearer "+c.Token)
	r.Header.Set("Accept", "application/vnd.github+json")
	r.Header.Set("User-Agent", "zoro.ai/"+Version)
	if body != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	resp, e := c.HTTPClient.Do(r)
	if e != nil {
		return nil, fmt.Errorf("%w: %v", app.ErrGitHub, e)
	}
	defer resp.Body.Close()
	b, e := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if e != nil {
		return nil, e
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: HTTP %d: %s", app.ErrGitHub, resp.StatusCode, safeMessage(b))
	}
	return b, nil
}
func safeMessage(b []byte) string {
	var v struct {
		Message string `json:"message"`
	}
	_ = json.Unmarshal(b, &v)
	if v.Message != "" {
		return v.Message
	}
	return strings.TrimSpace(string(b))
}
func (c *Client) GraphQL(ctx context.Context, q string, vars map[string]any, out any) error {
	payload, _ := json.Marshal(map[string]any{"query": q, "variables": vars})
	b, e := c.request(ctx, "POST", c.GraphQLURL, payload)
	if e != nil {
		return e
	}
	var env struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if e = json.Unmarshal(b, &env); e != nil {
		return fmt.Errorf("%w: decode response: %v", app.ErrGitHub, e)
	}
	if len(env.Errors) > 0 {
		return fmt.Errorf("%w: GraphQL: %s", app.ErrGitHub, env.Errors[0].Message)
	}
	return json.Unmarshal(env.Data, out)
}
func (c *Client) VerifyRepository(ctx context.Context, owner, repo string) error {
	_, e := c.request(ctx, "GET", fmt.Sprintf("%s/repos/%s/%s", c.RESTURL, owner, repo), nil)
	return e
}

func (c *Client) ListIssueComments(ctx context.Context, owner, repo string, issue int) ([]IssueComment, error) {
	var comments []IssueComment
	base := fmt.Sprintf("%s/repos/%s/%s/issues/%d/comments", c.RESTURL, url.PathEscape(owner), url.PathEscape(repo), issue)
	for page := 1; ; page++ {
		b, e := c.request(ctx, "GET", fmt.Sprintf("%s?per_page=100&page=%d", base, page), nil)
		if e != nil {
			return nil, e
		}
		var batch []IssueComment
		if e = json.Unmarshal(b, &batch); e != nil {
			return nil, fmt.Errorf("%w: decode issue comments: %v", app.ErrGitHub, e)
		}
		comments = append(comments, batch...)
		if len(batch) < 100 {
			return comments, nil
		}
	}
}

func (c *Client) CreateIssueComment(ctx context.Context, owner, repo string, issue int, body string) error {
	payload, e := json.Marshal(IssueComment{Body: body})
	if e != nil {
		return e
	}
	endpoint := fmt.Sprintf("%s/repos/%s/%s/issues/%d/comments", c.RESTURL, url.PathEscape(owner), url.PathEscape(repo), issue)
	_, e = c.request(ctx, "POST", endpoint, payload)
	return e
}

const projectQuery = `query($owner:String!,$repo:String!,$number:Int!){repository(owner:$owner,name:$repo){owner{__typename ... on Organization{projectV2(number:$number){...P}} ... on User{projectV2(number:$number){...P}}}}} fragment P on ProjectV2{id title fields(first:100){nodes{... on ProjectV2SingleSelectField{id name options{id name}}}} items(first:100){nodes{id content{... on Issue{id number title body repository{nameWithOwner}} ... on PullRequest{id number title body repository{nameWithOwner}} ... on DraftIssue{id title body}} fieldValues(first:20){nodes{... on ProjectV2ItemFieldSingleSelectValue{name field{... on ProjectV2SingleSelectField{name}}}}}}}}`

func (c *Client) Project(ctx context.Context, cfg config.GitHubConfig) (Project, error) {
	var d struct {
		Repository *struct {
			Owner struct {
				Type    string       `json:"__typename"`
				Project *projectNode `json:"projectV2"`
			} `json:"owner"`
		} `json:"repository"`
	}
	if e := c.GraphQL(ctx, projectQuery, map[string]any{"owner": cfg.Owner, "repo": cfg.Repo, "number": cfg.ProjectNumber}, &d); e != nil {
		return Project{}, e
	}
	if d.Repository == nil {
		return Project{}, fmt.Errorf("%w: repository %s/%s was not found", app.ErrProject, cfg.Owner, cfg.Repo)
	}
	n := d.Repository.Owner.Project
	if n == nil {
		return Project{}, fmt.Errorf("%w: GitHub repository owner %q is an %s, but project %d was not found or is not accessible", app.ErrProject, cfg.Owner, d.Repository.Owner.Type, cfg.ProjectNumber)
	}
	p := Project{ID: n.ID, Title: n.Title, StatusOptions: map[string]string{}}
	for _, f := range n.Fields.Nodes {
		if f.Name == cfg.StatusField {
			p.StatusFieldID = f.ID
			for _, o := range f.Options {
				p.StatusOptions[o.Name] = o.ID
			}
		}
	}
	if p.StatusFieldID == "" {
		return Project{}, fmt.Errorf("%w: status field %q not found", app.ErrProject, cfg.StatusField)
	}
	required := []string{cfg.Statuses.Backlog, cfg.Statuses.Ready, cfg.Statuses.Implementing, cfg.Statuses.Review, cfg.Statuses.Done}
	for _, x := range required {
		if p.StatusOptions[x] == "" {
			return Project{}, fmt.Errorf("%w: required status option %q not found", app.ErrProject, x)
		}
	}
	for i, n := range n.Items.Nodes {
		it := ProjectItem{ID: n.ID, ContentID: n.Content.ID, IssueNumber: n.Content.Number, Title: n.Content.Title, Body: n.Content.Body, Repository: n.Content.Repository.NameWithOwner, Position: i}
		for _, v := range n.FieldValues.Nodes {
			if v.Field.Name == cfg.StatusField {
				it.Status = v.Name
			}
		}
		p.Items = append(p.Items, it)
	}
	return p, nil
}

type projectNode struct {
	ID, Title string
	Fields    struct {
		Nodes []struct {
			ID, Name string
			Options  []struct{ ID, Name string }
		}
	}
	Items struct {
		Nodes []struct {
			ID      string
			Content struct {
				ID          string
				Number      int
				Title, Body string
				Repository  struct{ NameWithOwner string }
			}
			FieldValues struct {
				Nodes []struct {
					Name  string
					Field struct{ Name string }
				}
			}
		}
	}
}

func (p Project) Ready(status string) []ProjectItem {
	var out []ProjectItem
	for _, x := range p.Items {
		if x.Status == status {
			out = append(out, x)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Position < out[j].Position })
	return out
}

const updateMutation = `mutation($project:ID!,$item:ID!,$field:ID!,$option:String!){updateProjectV2ItemFieldValue(input:{projectId:$project,itemId:$item,fieldId:$field,value:{singleSelectOptionId:$option}}){projectV2Item{id}}}`

func (c *Client) UpdateStatus(ctx context.Context, p Project, itemID, status string) error {
	opt := p.StatusOptions[status]
	if opt == "" {
		return fmt.Errorf("%w: unknown status %q", app.ErrProject, status)
	}
	var out any
	return c.GraphQL(ctx, updateMutation, map[string]any{"project": p.ID, "item": itemID, "field": p.StatusFieldID, "option": opt}, &out)
}
