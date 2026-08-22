package github

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
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
