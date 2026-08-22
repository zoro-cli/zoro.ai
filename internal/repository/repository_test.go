package repository

import "testing"

func TestParseRemote(t *testing.T) {
	for _, in := range []string{"https://github.com/acme/app.git", "git@github.com:acme/app.git", "ssh://git@github.com/acme/app.git"} {
		o, r, e := ParseRemote(in)
		if e != nil || o != "acme" || r != "app" {
			t.Fatalf("%q => %q %q %v", in, o, r, e)
		}
	}
}
func TestBranchName(t *testing.T) {
	if got := BranchName("zoro", 142, "Add refresh token rotation"); got != "zoro/142-add-refresh-token-rotation" {
		t.Fatal(got)
	}
}
func TestExcluded(t *testing.T) {
	for _, p := range []string{".env", "config/.env.local", "id_rsa", "cert.pem", "vendor/x.go", "node_modules/a.js", "credentials.json"} {
		if !excluded(p) {
			t.Errorf("should exclude %s", p)
		}
	}
	if excluded("internal/auth/auth.go") {
		t.Fatal("excluded normal source")
	}
}
