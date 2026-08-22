package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zoro-cli/zoro.ai/internal/config"
)

func TestOrganizationProject(t *testing.T) {
	response := `{"data":{"repository":{"owner":{"__typename":"Organization","projectV2":{"id":"P1","title":"Roadmap","fields":{"nodes":[{"id":"F1","name":"Status","options":[{"id":"1","name":"Backlog"},{"id":"2","name":"Ready"},{"id":"3","name":"In progress"},{"id":"4","name":"In review"},{"id":"5","name":"Done"}]}]},"items":{"nodes":[{"id":"I1","content":{"id":"C1","number":7,"title":"Work","body":"body","repository":{"nameWithOwner":"acme/app"}},"fieldValues":{"nodes":[{"name":"Ready","field":{"name":"Status"}}]}}]}}}}}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer token" {
			t.Error("missing auth")
		}
		fmt.Fprint(w, response)
	}))
	defer srv.Close()
	c := New("token")
	c.GraphQLURL = srv.URL
	p, err := c.Project(context.Background(), config.Default("acme", "app", 1).GitHub)
	if err != nil {
		t.Fatal(err)
	}
	ready := p.Ready("Ready")
	if len(ready) != 1 || ready[0].IssueNumber != 7 {
		t.Fatalf("%+v", p)
	}
}

func TestIssueCommentsPaginationAndCreation(t *testing.T) {
	var posted string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer token" {
			t.Error("missing auth")
		}
		if r.URL.Path != "/repos/acme/app/issues/7/comments" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		switch r.Method {
		case http.MethodGet:
			if r.URL.Query().Get("per_page") != "100" {
				t.Errorf("missing page size")
			}
			if r.URL.Query().Get("page") == "1" {
				comments := make([]IssueComment, 100)
				for i := range comments {
					comments[i].Body = fmt.Sprintf("comment %d", i)
				}
				_ = json.NewEncoder(w).Encode(comments)
				return
			}
			fmt.Fprint(w, `[{"body":"marked"}]`)
		case http.MethodPost:
			b, _ := io.ReadAll(r.Body)
			var comment IssueComment
			if e := json.Unmarshal(b, &comment); e != nil {
				t.Fatal(e)
			}
			posted = comment.Body
			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, `{}`)
		default:
			t.Fatalf("unexpected method %s", r.Method)
		}
	}))
	defer srv.Close()
	c := New("token")
	c.RESTURL = srv.URL
	comments, e := c.ListIssueComments(context.Background(), "acme", "app", 7)
	if e != nil || len(comments) != 101 || comments[100].Body != "marked" {
		t.Fatalf("comments=%d error=%v", len(comments), e)
	}
	if e = c.CreateIssueComment(context.Background(), "acme", "app", 7, "handoff"); e != nil {
		t.Fatal(e)
	}
	if posted != "handoff" {
		t.Fatalf("posted %q", posted)
	}
}

func TestIssueCommentsErrorsAndCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"denied"}`, http.StatusUnauthorized)
	}))
	defer srv.Close()
	c := New("secret-token")
	c.RESTURL = srv.URL
	_, e := c.ListIssueComments(context.Background(), "acme", "app", 7)
	if e == nil || !strings.Contains(e.Error(), "denied") || strings.Contains(e.Error(), "secret-token") {
		t.Fatalf("unexpected error %v", e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, e = c.ListIssueComments(ctx, "acme", "app", 7)
	if e == nil || !strings.Contains(e.Error(), "context canceled") {
		t.Fatalf("unexpected cancellation error %v", e)
	}
}
