// wrapper is the SmoothNAS-side registration shim around the upstream
// GitHub Actions runner (actions/runner). The runner tarball bundles
// config.sh + run.sh, but neither understands SmoothNAS' lifecycle:
// containers come up with env-injected config and shut down on
// SIGTERM, and we want the runner to deregister itself from GitHub
// cleanly when that happens.
//
// On start the wrapper:
//
//  1. Reads GH_REPO_URL, GH_RUNNER_TOKEN, GH_RUNNER_LABELS,
//     GH_RUNNER_GROUP from the environment.
//  2. Detects whether GH_RUNNER_TOKEN is a personal access token
//     (PAT, prefix ghp_ or github_pat_) or already a registration
//     token. PATs let the wrapper exchange tokens itself, so a
//     restart re-registers without operator intervention.
//  3. For PATs, calls GitHub's API to fetch a registration token:
//     - repo URL  → POST /repos/{owner}/{repo}/actions/runners/registration-token
//     - org URL   → POST /orgs/{org}/actions/runners/registration-token
//  4. Runs ./config.sh with the resolved registration token,
//     labels, group, and a runner name keyed off $HOSTNAME.
//  5. Traps SIGTERM/SIGINT. On receipt, signals run.sh to wind down,
//     fetches a removal token (PATs only — registration tokens are
//     single-use and can't fetch a remove token), runs
//     ./config.sh remove, and exits.
//  6. Otherwise execs ./run.sh.
//
// The runner's working directory is the wrapper's working directory:
// the Dockerfile sets WORKDIR /home/runner where the tarball is
// extracted.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"strings"
	"syscall"
	"time"
)

const (
	defaultAPIBase    = "https://api.github.com"
	defaultRunnerHome = "/home/runner"
)

func main() {
	repoURL := os.Getenv("GH_REPO_URL")
	token := os.Getenv("GH_RUNNER_TOKEN")
	labels := envOr("GH_RUNNER_LABELS", "self-hosted,linux,x64,smoothnas")
	group := envOr("GH_RUNNER_GROUP", "default")
	apiBase := envOr("GH_API_BASE", defaultAPIBase)
	runnerHome := envOr("RUNNER_HOME", defaultRunnerHome)

	if repoURL == "" {
		log.Fatal("GH_REPO_URL is empty; refusing to start without a target repo or org")
	}
	if token == "" {
		log.Fatal("GH_RUNNER_TOKEN is empty; refusing to start without a registration credential")
	}

	scope, err := parseScope(repoURL)
	if err != nil {
		log.Fatalf("parse GH_REPO_URL: %v", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	tokenKind := classifyToken(token)
	log.Printf("registering runner: scope=%s token=%s labels=%q", scope, tokenKind, labels)

	regToken, err := resolveRegistrationToken(ctx, http.DefaultClient, apiBase, scope, token, tokenKind)
	if err != nil {
		log.Fatalf("resolve registration token: %v", err)
	}

	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "smoothnas-runner"
	}
	runnerName := "smoothnas-" + hostname

	configArgs := buildConfigArgs(repoURL, regToken, labels, group, runnerName, scope.IsOrg())
	if err := runScript(ctx, runnerHome, "./config.sh", configArgs); err != nil {
		log.Fatalf("config.sh: %v", err)
	}
	log.Printf("registered as %q", runnerName)

	// run.sh is a long-running process; we wrap it in a child and
	// proxy SIGTERM through. On exit we attempt to deregister.
	runErr := runRunSh(ctx, runnerHome)

	// Deregister regardless of run.sh exit status. The cancellation
	// path (SIGTERM) is the common case; an unprompted run.sh exit
	// also goes through here so we don't leave a stale registration
	// behind on a crash-and-restart.
	deregister(context.Background(), http.DefaultClient, runnerHome, apiBase, scope, token, tokenKind)

	if runErr != nil && !errors.Is(runErr, context.Canceled) {
		log.Printf("run.sh exited: %v", runErr)
		os.Exit(1)
	}
}

// scope identifies whether GH_REPO_URL targets a single repo or an
// org. The GitHub API uses different endpoints for each.
type scope struct {
	owner string
	repo  string // empty for org-scoped runners
}

func (s scope) IsOrg() bool { return s.repo == "" }

func (s scope) String() string {
	if s.repo == "" {
		return "org:" + s.owner
	}
	return "repo:" + s.owner + "/" + s.repo
}

// parseScope accepts either https://github.com/<owner>/<repo> or
// https://github.com/<owner>. Trailing slashes and .git suffixes are
// tolerated. We do not support GitHub Enterprise hostnames in v1;
// the parent doc explicitly defers that.
func parseScope(raw string) (scope, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return scope{}, fmt.Errorf("parse url: %w", err)
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return scope{}, fmt.Errorf("expected http(s) scheme, got %q", u.Scheme)
	}
	if u.Host != "github.com" && u.Host != "www.github.com" {
		return scope{}, fmt.Errorf("only github.com is supported in v1, got host %q", u.Host)
	}
	// Reject consecutive slashes ("//repo", "owner//", etc.) — these
	// usually indicate a typo'd URL where one segment is missing, and
	// silently treating them as org-scoped runners against the wrong
	// owner is worse than failing fast.
	if strings.Contains(u.Path, "//") {
		return scope{}, fmt.Errorf("malformed path %q (consecutive slashes)", u.Path)
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	switch len(parts) {
	case 1:
		if parts[0] == "" {
			return scope{}, errors.New("missing owner in URL path")
		}
		return scope{owner: parts[0]}, nil
	case 2:
		owner, repo := parts[0], strings.TrimSuffix(parts[1], ".git")
		if owner == "" || repo == "" {
			return scope{}, errors.New("empty owner or repo segment")
		}
		return scope{owner: owner, repo: repo}, nil
	default:
		return scope{}, fmt.Errorf("unexpected path %q; want /owner or /owner/repo", u.Path)
	}
}

// classifyToken returns "pat" if the supplied secret looks like a
// GitHub personal access token, or "regtoken" otherwise. Detection
// is heuristic — GitHub registration tokens have no published prefix
// (they're opaque ~30-char base32-ish strings) but PATs carry stable
// ghp_ / github_pat_ prefixes documented at
// https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/about-authentication-to-github#githubs-token-formats
func classifyToken(token string) string {
	switch {
	case strings.HasPrefix(token, "ghp_"),
		strings.HasPrefix(token, "github_pat_"),
		strings.HasPrefix(token, "ghs_"): // server-to-server (GitHub App installation) tokens — also work for the registration-token endpoint
		return "pat"
	default:
		return "regtoken"
	}
}

// resolveRegistrationToken returns a usable runner registration
// token. For PATs we POST to the registration-token endpoint to mint
// one; for direct registration tokens we pass the supplied value
// through unchanged.
func resolveRegistrationToken(ctx context.Context, client *http.Client, apiBase string, sc scope, token, kind string) (string, error) {
	if kind != "pat" {
		return token, nil
	}
	return mintRegistrationToken(ctx, client, apiBase, sc, token)
}

// mintRegistrationToken calls GitHub's runner registration-token
// endpoint, which mints a fresh ~1h registration token for the
// configured scope. The PAT must have actions:write (repo scope) or
// admin:org scope (org scope).
func mintRegistrationToken(ctx context.Context, client *http.Client, apiBase string, sc scope, pat string) (string, error) {
	endpoint, err := registrationTokenEndpoint(apiBase, sc)
	if err != nil {
		return "", err
	}
	return ghAPIPostToken(ctx, client, endpoint, pat)
}

// mintRemoveToken fetches a one-shot deregistration token. Used in
// the SIGTERM path so config.sh remove can deregister the runner
// from GitHub cleanly.
func mintRemoveToken(ctx context.Context, client *http.Client, apiBase string, sc scope, pat string) (string, error) {
	endpoint, err := removeTokenEndpoint(apiBase, sc)
	if err != nil {
		return "", err
	}
	return ghAPIPostToken(ctx, client, endpoint, pat)
}

func registrationTokenEndpoint(apiBase string, sc scope) (string, error) {
	return ghEndpoint(apiBase, sc, "registration-token")
}

func removeTokenEndpoint(apiBase string, sc scope) (string, error) {
	return ghEndpoint(apiBase, sc, "remove-token")
}

func ghEndpoint(apiBase string, sc scope, tokenAction string) (string, error) {
	base, err := url.Parse(apiBase)
	if err != nil {
		return "", fmt.Errorf("parse api base: %w", err)
	}
	var p string
	if sc.IsOrg() {
		p = path.Join(base.Path, "orgs", sc.owner, "actions", "runners", tokenAction)
	} else {
		p = path.Join(base.Path, "repos", sc.owner, sc.repo, "actions", "runners", tokenAction)
	}
	out := *base
	out.Path = p
	return out.String(), nil
}

// ghAPIPostToken POSTs against a GitHub /actions/runners/*-token
// endpoint and returns the minted token field. Both registration
// and remove endpoints share the same response shape.
func ghAPIPostToken(ctx context.Context, client *http.Client, endpoint, pat string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+pat)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "smoothnas-plugin-gh-runner/0.1")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("github api %s: %d %s", endpoint, resp.StatusCode, bytes.TrimSpace(body))
	}
	var out struct {
		Token     string `json:"token"`
		ExpiresAt string `json:"expires_at"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	if out.Token == "" {
		return "", fmt.Errorf("github api response missing token field; body=%s", string(body))
	}
	return out.Token, nil
}

// buildConfigArgs constructs the argv passed to ./config.sh. Org
// scope adds --runnergroup; repo scope ignores it (GitHub rejects
// the flag at repo level).
func buildConfigArgs(repoURL, regToken, labels, group, name string, isOrg bool) []string {
	args := []string{
		"--url", repoURL,
		"--token", regToken,
		"--labels", labels,
		"--name", name,
		"--unattended",
		"--replace",
	}
	if isOrg {
		args = append(args, "--runnergroup", group)
	}
	return args
}

// runScript runs an actions/runner script (config.sh / config.sh
// remove) to completion, streaming stdout/stderr to the wrapper's
// own. Exits non-zero are returned as errors.
func runScript(ctx context.Context, dir, script string, args []string) error {
	cmd := exec.CommandContext(ctx, script, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// runRunSh starts ./run.sh and waits for it to exit. SIGTERM/SIGINT
// to the wrapper propagate through ctx; CommandContext sends SIGKILL
// if run.sh ignores it, which it shouldn't — actions/runner handles
// SIGTERM by completing the in-flight job and exiting.
func runRunSh(ctx context.Context, dir string) error {
	cmd := exec.CommandContext(ctx, "./run.sh")
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	// CommandContext's default is SIGKILL on cancel, which would
	// strand a job. Set Cancel to send SIGTERM instead so the runner
	// drains gracefully.
	cmd.Cancel = func() error {
		if cmd.Process != nil {
			return cmd.Process.Signal(syscall.SIGTERM)
		}
		return nil
	}
	cmd.WaitDelay = 60 * time.Second
	return cmd.Run()
}

// deregister attempts a graceful runner removal from GitHub. We do
// this on shutdown regardless of cause: SIGTERM (most common) or
// run.sh exiting on its own. For PATs we mint a fresh remove token
// and call config.sh remove; for direct registration tokens we
// can't (the API endpoint requires a PAT and the registration token
// is single-use), so we log and exit, leaving the operator to clean
// up via the GitHub UI.
//
// Never propagates an error: best-effort. We have already accepted
// the shutdown signal; we cannot block on network failures.
func deregister(ctx context.Context, client *http.Client, runnerHome, apiBase string, sc scope, token, kind string) {
	if kind != "pat" {
		log.Printf("registration-token mode: skipping deregister; clean stale entries from GitHub UI if needed")
		return
	}
	// Cap the deregister at 30s so the container doesn't sit in
	// "stopping" forever on a flaky network.
	dctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	remToken, err := mintRemoveToken(dctx, client, apiBase, sc, token)
	if err != nil {
		log.Printf("mint remove token: %v (leaving runner registered)", err)
		return
	}
	if err := runScript(dctx, runnerHome, "./config.sh", []string{"remove", "--token", remToken}); err != nil {
		log.Printf("config.sh remove: %v", err)
		return
	}
	log.Printf("deregistered cleanly")
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
