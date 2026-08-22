package gitlab

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zoro-cli/zoro.ai/internal/config"
)

func TestProjectAndComments(t *testing.T) {
	var posted bool
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("PRIVATE-TOKEN") != "secret" {
			t.Error("missing token header")
		}
		switch {
		case strings.HasSuffix(r.URL.EscapedPath(), "/projects/group%2Fsub%2Fapp"):
			w.Write([]byte(`{"id":1,"path_with_namespace":"group/sub/app"}`))
		case strings.HasSuffix(r.URL.Path, "/boards/7"):
			w.Write([]byte(`{"id":7,"name":"Development","lists":[{"id":1,"label":{"name":"Backlog"}},{"id":2,"label":{"name":"Ready"}},{"id":3,"label":{"name":"Doing"}},{"id":4,"label":{"name":"Review"}},{"id":5,"label":{"name":"Done"}}]}`))
		case strings.Contains(r.URL.Path, "/lists/2/issues"):
			w.Write([]byte(`[{"id":20,"iid":9,"title":"Work","description":"Body","relative_position":4}]`))
		case strings.Contains(r.URL.Path, "/lists/") && strings.HasSuffix(r.URL.Path, "/issues"):
			w.Write([]byte(`[]`))
		case strings.HasSuffix(r.URL.Path, "/issues/9/notes") && r.Method == "GET":
			w.Write([]byte(`[{"body":"existing"}]`))
		case strings.HasSuffix(r.URL.Path, "/issues/9/notes") && r.Method == "POST":
			posted = true
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{}`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
			http.NotFound(w, r)
		}
	}))
	defer s.Close()
	cfg := config.GitLabConfig{Project: "group/sub/app", BoardID: 7, Statuses: config.Statuses{Backlog: "Backlog", Ready: "Ready", Implementing: "Doing", Review: "Review", Done: "Done"}}
	c := New("secret", s.URL)
	p, e := c.Project(context.Background(), cfg)
	if e != nil || len(p.Items) != 1 || p.Items[0].IssueNumber != 9 {
		t.Fatalf("project=%+v error=%v", p, e)
	}
	comments, e := c.ListIssueComments(context.Background(), cfg.Project, "", 9)
	if e != nil || len(comments) != 1 {
		t.Fatalf("comments=%+v error=%v", comments, e)
	}
	if e = c.CreateIssueComment(context.Background(), cfg.Project, "", 9, "new"); e != nil || !posted {
		t.Fatalf("posted=%t error=%v", posted, e)
	}
}
