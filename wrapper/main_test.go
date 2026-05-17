package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestParseScope(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		want    scope
		wantOrg bool
		wantErr bool
	}{
		{"repo", "https://github.com/owner/repo", scope{owner: "owner", repo: "repo"}, false, false},
		{"repo with .git", "https://github.com/owner/repo.git", scope{owner: "owner", repo: "repo"}, false, false},
		{"repo trailing slash", "https://github.com/owner/repo/", scope{owner: "owner", repo: "repo"}, false, false},
		{"org", "https://github.com/my-org", scope{owner: "my-org"}, true, false},
		{"org trailing slash", "https://github.com/my-org/", scope{owner: "my-org"}, true, false},
		{"www host", "https://www.github.com/owner/repo", scope{owner: "owner", repo: "repo"}, false, false},
		{"empty", "", scope{}, false, true},
		{"missing host", "https:///owner/repo", scope{}, false, true},
		{"non-github host", "https://gitlab.com/owner/repo", scope{}, false, true},
		{"too deep", "https://github.com/owner/repo/extra", scope{}, false, true},
		{"empty owner", "https://github.com//repo", scope{}, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseScope(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error, got scope=%v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("scope = %+v, want %+v", got, tc.want)
			}
			if got.IsOrg() != tc.wantOrg {
				t.Errorf("IsOrg = %v, want %v", got.IsOrg(), tc.wantOrg)
			}
		})
	}
}

func TestClassifyToken(t *testing.T) {
	cases := []struct {
		token string
		want  string
	}{
		{"ghp_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", "pat"},
		{"github_pat_11ABCDEFG_aaaaaaaaaaaaaaaaaaaaaa", "pat"},
		{"gho_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", "pat"},
		{"ghu_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", "pat"},
		{"ghs_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", "pat"},
		{"AAAAAAAAAAAAAAAAAAAAAAAAAAAA", "regtoken"},
		{"", "regtoken"},
		{"GHP_uppercased_does_not_count", "regtoken"},
	}
	for _, tc := range cases {
		t.Run(tc.token, func(t *testing.T) {
			if got := classifyToken(tc.token); got != tc.want {
				t.Errorf("classifyToken(%q) = %q, want %q", tc.token, got, tc.want)
			}
		})
	}
}

func TestGhEndpoint(t *testing.T) {
	cases := []struct {
		name        string
		apiBase     string
		sc          scope
		tokenAction string
		want        string
	}{
		{
			"repo registration",
			"https://api.github.com",
			scope{owner: "owner", repo: "repo"},
			"registration-token",
			"https://api.github.com/repos/owner/repo/actions/runners/registration-token",
		},
		{
			"repo remove",
			"https://api.github.com",
			scope{owner: "owner", repo: "repo"},
			"remove-token",
			"https://api.github.com/repos/owner/repo/actions/runners/remove-token",
		},
		{
			"org registration",
			"https://api.github.com",
			scope{owner: "my-org"},
			"registration-token",
			"https://api.github.com/orgs/my-org/actions/runners/registration-token",
		},
		{
			"api base with path prefix (GHE-style; v1 doesn't accept the URL but the helper still composes correctly)",
			"https://example.com/api/v3",
			scope{owner: "owner", repo: "repo"},
			"registration-token",
			"https://example.com/api/v3/repos/owner/repo/actions/runners/registration-token",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ghEndpoint(tc.apiBase, tc.sc, tc.tokenAction)
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestBuildConfigArgs(t *testing.T) {
	t.Run("repo scope omits runnergroup", func(t *testing.T) {
		got := buildConfigArgs("https://github.com/owner/repo", "REG", "self-hosted,smoothnas", "default", "smoothnas-host", false, false)
		want := []string{
			"--url", "https://github.com/owner/repo",
			"--token", "REG",
			"--labels", "self-hosted,smoothnas",
			"--name", "smoothnas-host",
			"--unattended",
			"--replace",
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})
	t.Run("ephemeral adds one-shot flag", func(t *testing.T) {
		got := buildConfigArgs("https://github.com/owner/repo", "REG", "self-hosted,smoothnas", "default", "smoothnas-host", false, true)
		want := []string{
			"--url", "https://github.com/owner/repo",
			"--token", "REG",
			"--labels", "self-hosted,smoothnas",
			"--name", "smoothnas-host",
			"--unattended",
			"--replace",
			"--ephemeral",
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})
	t.Run("org scope appends runnergroup", func(t *testing.T) {
		got := buildConfigArgs("https://github.com/my-org", "REG", "self-hosted,smoothnas", "default", "smoothnas-host", true, false)
		want := []string{
			"--url", "https://github.com/my-org",
			"--token", "REG",
			"--labels", "self-hosted,smoothnas",
			"--name", "smoothnas-host",
			"--unattended",
			"--replace",
			"--runnergroup", "default",
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})
}

func TestRunnerNameFromHostname(t *testing.T) {
	t.Run("shortens container id hostnames", func(t *testing.T) {
		got := runnerNameFromHostname("31df9337080579e1f4c3c75029d8debe0570018afc174dbc5cc4bce8a7d3ea50")
		if got != "smoothnas-31df93370805" {
			t.Fatalf("name = %q", got)
		}
	})

	t.Run("caps long non-hex hostnames", func(t *testing.T) {
		got := runnerNameFromHostname(strings.Repeat("host", 30))
		if len(got) > maxRunnerNameLen {
			t.Fatalf("name length = %d, want <= %d: %q", len(got), maxRunnerNameLen, got)
		}
		if !strings.HasPrefix(got, runnerNamePrefix) {
			t.Fatalf("name = %q, missing prefix", got)
		}
	})

	t.Run("sanitizes invalid characters", func(t *testing.T) {
		got := runnerNameFromHostname(`bad/name:with*chars?`)
		if got != "smoothnas-bad-name-with-chars" {
			t.Fatalf("name = %q", got)
		}
	})

	t.Run("empty uses fallback", func(t *testing.T) {
		got := runnerNameFromHostname("  ")
		if got != "smoothnas-runner" {
			t.Fatalf("name = %q", got)
		}
	})
}

func TestRunnerNameWithSuffix(t *testing.T) {
	t.Run("appends suffix", func(t *testing.T) {
		got := runnerNameWithSuffix("smoothnas-host", "123-4")
		if got != "smoothnas-host-123-4" {
			t.Fatalf("name = %q", got)
		}
	})

	t.Run("caps length", func(t *testing.T) {
		got := runnerNameWithSuffix(runnerNamePrefix+strings.Repeat("a", 80), "1234567890")
		if len(got) > maxRunnerNameLen {
			t.Fatalf("name length = %d, want <= %d: %q", len(got), maxRunnerNameLen, got)
		}
		if !strings.HasSuffix(got, "-1234567890") {
			t.Fatalf("name = %q, missing suffix", got)
		}
	})
}

func TestRunnerConfigured(t *testing.T) {
	dir := t.TempDir()
	if runnerConfigured(dir) {
		t.Fatal("fresh directory should not be configured")
	}
	if err := os.WriteFile(dir+"/.runner", []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !runnerConfigured(dir) {
		t.Fatal("directory with .runner should be configured")
	}
}

func TestCleanupRunnerState(t *testing.T) {
	dir := t.TempDir()
	for _, rel := range []string{".runner", ".credentials", ".credentials_rsaparams", ".env", ".path", ".service"} {
		if err := os.WriteFile(dir+"/"+rel, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, rel := range []string{"_work/_actions", "_work/_temp", "_work/_tool"} {
		if err := os.MkdirAll(dir+"/"+rel+"/nested", 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := cleanupRunnerState(dir); err != nil {
		t.Fatalf("cleanupRunnerState: %v", err)
	}
	for _, rel := range []string{".runner", ".credentials", ".credentials_rsaparams", ".env", ".path", ".service", "_work/_actions", "_work/_temp", "_work/_tool"} {
		if _, err := os.Stat(dir + "/" + rel); !os.IsNotExist(err) {
			t.Fatalf("%s still exists or stat failed with %v", rel, err)
		}
	}
}

func TestMintRegistrationToken_Repo(t *testing.T) {
	var got struct {
		method string
		path   string
		auth   string
		accept string
		apiVer string
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.method = r.Method
		got.path = r.URL.Path
		got.auth = r.Header.Get("Authorization")
		got.accept = r.Header.Get("Accept")
		got.apiVer = r.Header.Get("X-GitHub-Api-Version")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"token":      "AAAA",
			"expires_at": "2099-01-01T00:00:00Z",
		})
	}))
	defer srv.Close()

	tok, err := mintRegistrationToken(context.Background(), srv.Client(), srv.URL, scope{owner: "owner", repo: "repo"}, "ghp_xxx")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if tok != "AAAA" {
		t.Errorf("token = %q want AAAA", tok)
	}
	if got.method != http.MethodPost {
		t.Errorf("method = %q want POST", got.method)
	}
	if got.path != "/repos/owner/repo/actions/runners/registration-token" {
		t.Errorf("path = %q", got.path)
	}
	if got.auth != "Bearer ghp_xxx" {
		t.Errorf("auth = %q", got.auth)
	}
	if !strings.Contains(got.accept, "github+json") {
		t.Errorf("accept = %q", got.accept)
	}
	if got.apiVer != "2022-11-28" {
		t.Errorf("api version header = %q", got.apiVer)
	}
}

func TestMintRegistrationToken_Org(t *testing.T) {
	var seenPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		_ = json.NewEncoder(w).Encode(map[string]any{"token": "BBBB"})
	}))
	defer srv.Close()

	tok, err := mintRegistrationToken(context.Background(), srv.Client(), srv.URL, scope{owner: "my-org"}, "ghp_xxx")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if tok != "BBBB" {
		t.Errorf("token = %q want BBBB", tok)
	}
	if seenPath != "/orgs/my-org/actions/runners/registration-token" {
		t.Errorf("path = %q", seenPath)
	}
}

func TestMintRegistrationToken_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"Bad credentials"}`))
	}))
	defer srv.Close()

	_, err := mintRegistrationToken(context.Background(), srv.Client(), srv.URL, scope{owner: "owner", repo: "repo"}, "ghp_xxx")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error = %q want 401 mention", err)
	}
}

func TestMintRegistrationToken_MissingTokenField(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	_, err := mintRegistrationToken(context.Background(), srv.Client(), srv.URL, scope{owner: "owner", repo: "repo"}, "ghp_xxx")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "missing token field") {
		t.Errorf("error = %q", err)
	}
}

func TestResolveRegistrationToken_PassthroughForRegToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("API should not be called when token is already a registration token")
	}))
	defer srv.Close()

	got, err := resolveRegistrationToken(context.Background(), srv.Client(), srv.URL, scope{owner: "owner", repo: "repo"}, "AAAA-direct-regtoken", "regtoken")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != "AAAA-direct-regtoken" {
		t.Errorf("got %q, want passthrough", got)
	}
}

func TestMintRemoveToken_HitsRemoveEndpoint(t *testing.T) {
	var seenPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		_ = json.NewEncoder(w).Encode(map[string]any{"token": "REM"})
	}))
	defer srv.Close()

	tok, err := mintRemoveToken(context.Background(), srv.Client(), srv.URL, scope{owner: "owner", repo: "repo"}, "ghp_xxx")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if tok != "REM" {
		t.Errorf("token = %q", tok)
	}
	if seenPath != "/repos/owner/repo/actions/runners/remove-token" {
		t.Errorf("path = %q", seenPath)
	}
}

func TestRemoveStaleGitHubRunnersDeletesOfflineSmoothNASRunners(t *testing.T) {
	var deletes []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/orgs/my-org/actions/runners":
			if r.URL.Query().Get("per_page") != "100" {
				t.Errorf("per_page = %q", r.URL.Query().Get("per_page"))
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"runners": []map[string]any{
					{"id": 1, "name": "smoothnas-dead", "status": "offline", "busy": true},
					{"id": 2, "name": "smoothnas-live", "status": "online", "busy": false},
					{"id": 3, "name": "other-dead", "status": "offline", "busy": false},
				},
			})
		case r.Method == http.MethodDelete:
			deletes = append(deletes, r.URL.Path)
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer srv.Close()

	err := removeStaleGitHubRunners(context.Background(), srv.Client(), srv.URL, scope{owner: "my-org"}, "ghp_xxx")
	if err != nil {
		t.Fatalf("removeStaleGitHubRunners: %v", err)
	}
	want := []string{"/orgs/my-org/actions/runners/1"}
	if !reflect.DeepEqual(deletes, want) {
		t.Fatalf("deletes = %v, want %v", deletes, want)
	}
}

func TestDeleteGitHubRunnerIgnoresAlreadyGone(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/repos/owner/repo/actions/runners/42" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	err := deleteGitHubRunner(context.Background(), srv.Client(), srv.URL, scope{owner: "owner", repo: "repo"}, "ghp_xxx", 42)
	if err != nil {
		t.Fatalf("deleteGitHubRunner: %v", err)
	}
}

func TestEnvOr(t *testing.T) {
	if got := envOr("DEFINITELY_NOT_SET_GHRUNNER", "fallback"); got != "fallback" {
		t.Errorf("got %q, want fallback", got)
	}
	t.Setenv("WRAPPER_TEST_KEY", "explicit")
	if got := envOr("WRAPPER_TEST_KEY", "fallback"); got != "explicit" {
		t.Errorf("got %q, want explicit", got)
	}
}

func TestEnvBool(t *testing.T) {
	if !envBool("DEFINITELY_NOT_SET_GHRUNNER_BOOL", true) {
		t.Fatal("unset true default returned false")
	}
	t.Setenv("WRAPPER_BOOL_TRUE", "yes")
	if !envBool("WRAPPER_BOOL_TRUE", false) {
		t.Fatal("yes should parse true")
	}
	t.Setenv("WRAPPER_BOOL_FALSE", "0")
	if envBool("WRAPPER_BOOL_FALSE", true) {
		t.Fatal("0 should parse false")
	}
	t.Setenv("WRAPPER_BOOL_BAD", "maybe")
	if !envBool("WRAPPER_BOOL_BAD", true) {
		t.Fatal("invalid should return default")
	}
}

func TestLoadConfigDefaultsToEphemeralWorkerWorkspace(t *testing.T) {
	t.Setenv("GH_REPO_URL", "https://github.com/owner/repo")
	t.Setenv("GH_RUNNER_TOKEN", "ghp_xxx")

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if cfg.bindWorkspace {
		t.Fatal("GH_RUNNER_BIND_WORKSPACE default should leave worker workspace in the ephemeral rootfs")
	}

	t.Setenv("GH_RUNNER_BIND_WORKSPACE", "true")
	cfg, err = loadConfig()
	if err != nil {
		t.Fatalf("loadConfig with bind workspace: %v", err)
	}
	if !cfg.bindWorkspace {
		t.Fatal("GH_RUNNER_BIND_WORKSPACE=true should bind the tier workspace into workers")
	}
}

func TestEnvInt(t *testing.T) {
	if got := envInt("DEFINITELY_NOT_SET_GHRUNNER_INT", 4); got != 4 {
		t.Fatalf("unset default = %d, want 4", got)
	}
	t.Setenv("WRAPPER_INT", "7")
	if got := envInt("WRAPPER_INT", 4); got != 7 {
		t.Fatalf("parsed = %d, want 7", got)
	}
	t.Setenv("WRAPPER_INT_BAD", "many")
	if got := envInt("WRAPPER_INT_BAD", 4); got != 4 {
		t.Fatalf("invalid = %d, want default", got)
	}
}

func TestHostMountSource(t *testing.T) {
	var inspect containerInspect
	inspect.Mounts = append(inspect.Mounts, struct {
		Source      string `json:"Source"`
		Destination string `json:"Destination"`
	}{Source: "/host/work", Destination: "/home/runner/_work"})

	got, err := hostMountSource(inspect, "/home/runner/_work")
	if err != nil {
		t.Fatal(err)
	}
	if got != "/host/work" {
		t.Fatalf("source = %q, want /host/work", got)
	}
}

func TestMintRegistrationToken_ContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Sleep past the test's context timeout — never reached.
		time.Sleep(2 * time.Second)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := mintRegistrationToken(ctx, srv.Client(), srv.URL, scope{owner: "owner", repo: "repo"}, "ghp_xxx")
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
}
