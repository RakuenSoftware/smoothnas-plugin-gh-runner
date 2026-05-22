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
//  2. Detects whether GH_RUNNER_TOKEN is a GitHub API token or already
//     a registration token. API tokens let the wrapper exchange tokens
//     itself, so a restart re-registers without operator intervention.
//  3. For API tokens, calls GitHub's API to fetch a registration token:
//     - repo URL  → POST /repos/{owner}/{repo}/actions/runners/registration-token
//     - org URL   → POST /orgs/{org}/actions/runners/registration-token
//  4. Runs ./config.sh with the resolved registration token,
//     labels, group, and a runner name keyed off $HOSTNAME.
//  5. Traps SIGTERM/SIGINT. On receipt, signals run.sh to wind down,
//     fetches a removal token (API tokens only — registration tokens are
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
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	defaultAPIBase    = "https://api.github.com"
	defaultRunnerHome = "/home/runner"
	defaultWorkers    = 1
	runnerNamePrefix  = "smoothnas-"
	maxRunnerNameLen  = 64
	recycleDelay      = 10 * time.Second
	defaultDockerHost = "unix:///var/run/docker.sock"
	hostRuntimeSocket = "/run/smoothnas-runtime/docker.sock"
	workerLabelKey    = "io.smoothnas.gh-runner.worker"
	staleSweepEvery   = 1 * time.Minute
	registrationGrace = 2 * time.Minute
)

var resolvConfPath = "/etc/resolv.conf"
var actionNodeBackupDir = "/usr/local/share/smoothnas-actions-node"
var actionNodeFallbackBackupDir = "/opt/smoothnas/actions-node"
var runExternalCommand = runCommand

type nodeRuntimeSpec struct {
	version string
	sha256  map[string]string
}

var actionNodeSpecs = map[string]nodeRuntimeSpec{
	"20": {
		version: "20.20.2",
		sha256: map[string]string{
			"arm":   "f704ce75d9a194c30c378049b516000e49612c2f046ac83c7435eb33ec2926f0",
			"arm64": "73093db209e4e9e09dd7d15a47aeaab1b74833830df03efa5f942a1122c5fa71",
			"x64":   "df770b2a6f130ed8627c9782c988fda9669fa23898329a61a871e32f965e007d",
		},
	},
	"24": {
		version: "24.15.0",
		sha256: map[string]string{
			"arm64": "f3d5a797b5d210ce8e2cb265544c8e482eaedcb8aa409a8b46da7e8595d0dda0",
			"x64":   "472655581fb851559730c48763e0c9d3bc25975c59d518003fc0849d3e4ba0f6",
		},
	},
}

type config struct {
	mode           string
	repoURL        string
	token          string
	tokenKind      string
	labels         string
	group          string
	apiBase        string
	runnerHome     string
	ephemeral      bool
	scope          scope
	workerCount    int
	workerCPUs     float64
	workerMemory   int64
	dockerHost     string
	workerImage    string
	bindWorkspace  bool
	dnsServers     []string
	workspaceRepos []string
}

func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatal(err)
	}
	if err := stabilizeContainerDNS(cfg.dnsServers); err != nil {
		log.Printf("stabilize container dns: %v", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	switch cfg.mode {
	case "controller":
		if cfg.tokenKind != "pat" {
			log.Fatal("controller mode requires a GitHub API token so workers can mint fresh registration tokens")
		}
		if err := runController(ctx, cfg); err != nil && !errors.Is(err, context.Canceled) {
			log.Fatal(err)
		}
	case "worker":
		if cfg.tokenKind != "pat" {
			log.Fatal("worker mode requires a GitHub API token so the runner can mint fresh registration tokens")
		}
		if err := ensureActionNodeRuntimes(cfg.runnerHome); err != nil {
			log.Fatal(err)
		}
		if err := ensureRunnerJobHooks(cfg.runnerHome); err != nil {
			log.Fatal(err)
		}
		if err := runEphemeralOnce(ctx, cfg); err != nil && !errors.Is(err, context.Canceled) {
			log.Fatal(err)
		}
	case "loop":
		if cfg.tokenKind != "pat" {
			log.Fatal("loop mode requires a GitHub API token so the wrapper can mint fresh registration tokens")
		}
		if err := ensureActionNodeRuntimes(cfg.runnerHome); err != nil {
			log.Fatal(err)
		}
		if err := ensureRunnerJobHooks(cfg.runnerHome); err != nil {
			log.Fatal(err)
		}
		runEphemeralLoop(ctx, cfg)
	case "persistent":
		if err := ensureActionNodeRuntimes(cfg.runnerHome); err != nil {
			log.Fatal(err)
		}
		if err := ensureRunnerJobHooks(cfg.runnerHome); err != nil {
			log.Fatal(err)
		}
		if err := runPersistent(ctx, cfg); err != nil {
			log.Fatal(err)
		}
	default:
		log.Fatalf("unsupported GH_RUNNER_MODE=%q; expected controller, worker, loop, or persistent", cfg.mode)
	}
}

func loadConfig() (config, error) {
	repoURL := os.Getenv("GH_REPO_URL")
	token := os.Getenv("GH_RUNNER_TOKEN")
	if repoURL == "" {
		return config{}, errors.New("GH_REPO_URL is empty; refusing to start without a target repo or org")
	}
	if token == "" {
		return config{}, errors.New("GH_RUNNER_TOKEN is empty; refusing to start without a registration credential")
	}

	sc, err := parseScope(repoURL)
	if err != nil {
		return config{}, fmt.Errorf("parse GH_REPO_URL: %w", err)
	}

	return config{
		mode:           envOr("GH_RUNNER_MODE", "controller"),
		repoURL:        repoURL,
		token:          token,
		tokenKind:      classifyToken(token),
		labels:         envOr("GH_RUNNER_LABELS", "self-hosted,linux,x64,smoothnas"),
		group:          envOr("GH_RUNNER_GROUP", "default"),
		apiBase:        envOr("GH_API_BASE", defaultAPIBase),
		runnerHome:     envOr("RUNNER_HOME", defaultRunnerHome),
		ephemeral:      envBool("GH_RUNNER_EPHEMERAL", true),
		scope:          sc,
		workerCount:    envInt("GH_RUNNER_WORKERS", defaultWorkers),
		workerCPUs:     envFloat("GH_RUNNER_CPUS", 0),
		workerMemory:   envBytes("GH_RUNNER_MEMORY", 0),
		dockerHost:     envOr("DOCKER_HOST", defaultDockerHost),
		workerImage:    os.Getenv("GH_RUNNER_WORKER_IMAGE"),
		bindWorkspace:  envBool("GH_RUNNER_BIND_WORKSPACE", false),
		dnsServers:     envList("GH_RUNNER_DNS_SERVERS", ""),
		workspaceRepos: envList("GH_RUNNER_WORKSPACE_REPOS", ""),
	}, nil
}

func stabilizeContainerDNS(servers []string) error {
	if len(servers) == 0 {
		return nil
	}
	var valid []string
	for _, server := range servers {
		if ip := net.ParseIP(server); ip != nil {
			valid = append(valid, server)
		}
	}
	if len(valid) == 0 {
		return nil
	}
	var b strings.Builder
	for _, server := range valid {
		fmt.Fprintf(&b, "nameserver %s\n", server)
	}
	b.WriteString("options timeout:1 attempts:2\n")
	return os.WriteFile(resolvConfPath, []byte(b.String()), 0o644)
}

func runPersistent(ctx context.Context, cfg config) error {
	log.Printf("registering persistent runner: scope=%s token=%s labels=%q", cfg.scope, cfg.tokenKind, cfg.labels)

	regToken, err := resolveRegistrationToken(ctx, http.DefaultClient, cfg.apiBase, cfg.scope, cfg.token, cfg.tokenKind)
	if err != nil {
		return fmt.Errorf("resolve registration token: %w", err)
	}

	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "smoothnas-runner"
	}
	runnerName := runnerNameFromHostname(hostname)

	if err := ensureRunnerWorkspaces(ctx, cfg); err != nil {
		log.Printf("precreate runner workspaces: %v", err)
	}

	if runnerConfigured(cfg.runnerHome) {
		log.Printf("runner already configured; starting existing registration")
	} else {
		configArgs := buildConfigArgs(cfg.repoURL, regToken, cfg.labels, cfg.group, runnerName, cfg.scope.IsOrg(), false)
		if err := runScript(ctx, cfg.runnerHome, "./config.sh", configArgs); err != nil {
			return fmt.Errorf("config.sh: %w", err)
		}
		log.Printf("registered as %q", runnerName)
	}

	runErr := runRunSh(ctx, cfg.runnerHome)

	// Deregister regardless of run.sh exit status. The cancellation
	// path (SIGTERM) is the common case; an unprompted run.sh exit
	// also goes through here so we don't leave a stale registration
	// behind on a crash-and-restart.
	deregister(context.Background(), http.DefaultClient, cfg.runnerHome, cfg.apiBase, cfg.scope, cfg.token, cfg.tokenKind)

	if runErr != nil && !errors.Is(runErr, context.Canceled) {
		return fmt.Errorf("run.sh exited: %w", runErr)
	}
	return nil
}

func ensureActionNodeRuntimes(runnerHome string) error {
	for _, major := range []string{"20", "24"} {
		dest := filepath.Join(runnerHome, "externals", "node"+major, "bin", "node")
		if executableFile(dest) {
			continue
		}
		if err := restoreActionNodeRuntime(major, dest); err != nil {
			return fmt.Errorf("restore node%s action runtime: %w", major, err)
		}
		log.Printf("restored node%s action runtime to %s", major, dest)
	}
	return nil
}

func restoreActionNodeRuntime(major, dest string) error {
	for _, backupDir := range []string{actionNodeBackupDir, actionNodeFallbackBackupDir} {
		src := filepath.Join(backupDir, "node"+major, "node")
		if executableFile(src) {
			return copyExecutable(src, dest)
		}
		if err := restoreActionNodeRuntimeFromChunks(filepath.Join(backupDir, "node"+major), dest); err == nil {
			return nil
		}
	}
	if strings.EqualFold(os.Getenv("GH_RUNNER_ALLOW_NODE_DOWNLOAD"), "true") {
		log.Printf("node%s backup runtime missing; downloading pinned runtime", major)
		return downloadActionNodeRuntime(context.Background(), major, dest)
	}
	log.Printf("node%s backup runtime missing; set GH_RUNNER_ALLOW_NODE_DOWNLOAD=true to fetch it at startup", major)
	return fmt.Errorf("node%s backup runtime missing", major)
}

func restoreActionNodeRuntimeFromChunks(dir, dest string) error {
	chunks, err := filepath.Glob(filepath.Join(dir, "node.part.*"))
	if err != nil {
		return err
	}
	if len(chunks) == 0 {
		return os.ErrNotExist
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	tmp := dest + ".tmp"
	_ = os.Remove(tmp)
	out, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o755)
	if err != nil {
		return err
	}
	for _, chunk := range chunks {
		in, err := os.Open(chunk)
		if err != nil {
			_ = out.Close()
			_ = os.Remove(tmp)
			return err
		}
		_, copyErr := io.Copy(out, in)
		closeErr := in.Close()
		if copyErr != nil {
			_ = out.Close()
			_ = os.Remove(tmp)
			return copyErr
		}
		if closeErr != nil {
			_ = out.Close()
			_ = os.Remove(tmp)
			return closeErr
		}
	}
	closeErr := out.Close()
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	if err := os.Chmod(tmp, 0o755); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	_ = os.Remove(dest)
	return os.Rename(tmp, dest)
}

func downloadActionNodeRuntime(ctx context.Context, major, dest string) error {
	spec, ok := actionNodeSpecs[major]
	if !ok {
		return fmt.Errorf("unsupported node runtime major %q", major)
	}
	nodeArch, err := nodeRuntimeArch(runtime.GOARCH)
	if err != nil {
		return err
	}
	sha256, ok := spec.sha256[nodeArch]
	if !ok {
		return fmt.Errorf("node%s does not publish linux-%s binaries", major, nodeArch)
	}

	tmpDir, err := os.MkdirTemp("", "smoothnas-node-"+major+"-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	archive := filepath.Join(tmpDir, "node.tar.xz")
	extractDir := fmt.Sprintf("node-v%s-linux-%s", spec.version, nodeArch)
	url := fmt.Sprintf("https://nodejs.org/dist/v%s/%s.tar.xz", spec.version, extractDir)
	if err := runExternalCommand(ctx, tmpDir, "curl", "-fsSLo", archive, url); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "node.sha256"), []byte(sha256+"  node.tar.xz\n"), 0o644); err != nil {
		return err
	}
	if err := runExternalCommand(ctx, tmpDir, "sha256sum", "-c", "node.sha256"); err != nil {
		return err
	}
	if err := runExternalCommand(ctx, tmpDir, "tar", "-xJf", archive); err != nil {
		return err
	}
	return copyExecutable(filepath.Join(tmpDir, extractDir, "bin", "node"), dest)
}

func nodeRuntimeArch(goarch string) (string, error) {
	switch goarch {
	case "amd64":
		return "x64", nil
	case "arm64":
		return "arm64", nil
	case "arm":
		return "armv7l", nil
	default:
		return "", fmt.Errorf("unsupported architecture %q", goarch)
	}
}

func runCommand(ctx context.Context, dir, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}

func executableFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	return info.Mode()&0o111 != 0
}

func copyExecutable(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	tmp := dest + ".tmp"
	_ = os.Remove(tmp)
	out, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o755)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	if err := os.Chmod(tmp, 0o755); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	_ = os.Remove(dest)
	return os.Rename(tmp, dest)
}

func runEphemeralOnce(ctx context.Context, cfg config) error {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "smoothnas-runner"
	}
	baseName := runnerNameFromHostname(hostname)
	runnerName := runnerNameWithSuffix(baseName, fmt.Sprintf("-%d", time.Now().Unix()))

	if err := cleanupRunnerState(cfg.runnerHome); err != nil {
		log.Printf("cleanup before worker start: %v", err)
	}
	if err := ensureRunnerWorkspaces(ctx, cfg); err != nil {
		log.Printf("precreate runner workspaces: %v", err)
	}

	regToken, err := mintRegistrationToken(ctx, http.DefaultClient, cfg.apiBase, cfg.scope, cfg.token)
	if err != nil {
		return fmt.Errorf("mint registration token: %w", err)
	}

	configArgs := buildConfigArgs(cfg.repoURL, regToken, cfg.labels, cfg.group, runnerName, cfg.scope.IsOrg(), true)
	if err := runScript(ctx, cfg.runnerHome, "./config.sh", configArgs); err != nil {
		return fmt.Errorf("config.sh: %w", err)
	}
	log.Printf("registered ephemeral runner %q", runnerName)

	runErr := runRunSh(ctx, cfg.runnerHome)
	if ctx.Err() != nil {
		deregister(context.Background(), http.DefaultClient, cfg.runnerHome, cfg.apiBase, cfg.scope, cfg.token, cfg.tokenKind)
		_ = cleanupRunnerState(cfg.runnerHome)
		return ctx.Err()
	}
	if runErr != nil {
		deregister(context.Background(), http.DefaultClient, cfg.runnerHome, cfg.apiBase, cfg.scope, cfg.token, cfg.tokenKind)
		_ = cleanupRunnerState(cfg.runnerHome)
		return fmt.Errorf("run.sh exited: %w", runErr)
	}

	log.Printf("ephemeral runner %q completed one job", runnerName)
	if err := cleanupRunnerState(cfg.runnerHome); err != nil {
		log.Printf("cleanup after worker exit: %v", err)
	}
	return nil
}

func runEphemeralLoop(ctx context.Context, cfg config) {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "smoothnas-runner"
	}
	baseName := runnerNameFromHostname(hostname)
	log.Printf("starting ephemeral runner loop: scope=%s labels=%q base_name=%q", cfg.scope, cfg.labels, baseName)

	for cycle := 1; ; cycle++ {
		if ctx.Err() != nil {
			return
		}
		if err := cleanupRunnerState(cfg.runnerHome); err != nil {
			log.Printf("cleanup before cycle %d: %v", cycle, err)
		}
		if err := ensureRunnerWorkspaces(ctx, cfg); err != nil {
			log.Printf("precreate runner workspaces: %v", err)
		}

		regToken, err := mintRegistrationToken(ctx, http.DefaultClient, cfg.apiBase, cfg.scope, cfg.token)
		if err != nil {
			log.Printf("mint registration token: %v", err)
			if !sleepOrDone(ctx, recycleDelay) {
				return
			}
			continue
		}

		runnerName := runnerNameWithSuffix(baseName, fmt.Sprintf("-%d-%d", time.Now().Unix(), cycle))
		configArgs := buildConfigArgs(cfg.repoURL, regToken, cfg.labels, cfg.group, runnerName, cfg.scope.IsOrg(), true)
		if err := runScript(ctx, cfg.runnerHome, "./config.sh", configArgs); err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("config.sh: %v", err)
			if !sleepOrDone(ctx, recycleDelay) {
				return
			}
			continue
		}
		log.Printf("registered ephemeral runner %q", runnerName)

		runErr := runRunSh(ctx, cfg.runnerHome)
		if ctx.Err() != nil {
			deregister(context.Background(), http.DefaultClient, cfg.runnerHome, cfg.apiBase, cfg.scope, cfg.token, cfg.tokenKind)
			_ = cleanupRunnerState(cfg.runnerHome)
			return
		}
		if runErr != nil {
			log.Printf("ephemeral run.sh exited: %v", runErr)
			deregister(context.Background(), http.DefaultClient, cfg.runnerHome, cfg.apiBase, cfg.scope, cfg.token, cfg.tokenKind)
			_ = cleanupRunnerState(cfg.runnerHome)
			if !sleepOrDone(ctx, recycleDelay) {
				return
			}
			continue
		}

		log.Printf("ephemeral runner %q completed one job; recycling", runnerName)
		if err := cleanupRunnerState(cfg.runnerHome); err != nil {
			log.Printf("cleanup after cycle %d: %v", cycle, err)
		}
	}
}

func runnerNameFromHostname(hostname string) string {
	namePart := sanitizeRunnerNamePart(hostname)
	if namePart == "" {
		namePart = "runner"
	}
	if isHexID(namePart) && len(namePart) > 12 {
		namePart = namePart[:12]
	}
	maxPartLen := maxRunnerNameLen - len(runnerNamePrefix)
	if len(namePart) > maxPartLen {
		namePart = namePart[:maxPartLen]
	}
	return runnerNamePrefix + namePart
}

func runnerNameWithSuffix(baseName, suffix string) string {
	baseName = strings.TrimSpace(baseName)
	suffix = sanitizeRunnerNamePart(suffix)
	if suffix == "" {
		return baseName
	}
	if !strings.HasPrefix(suffix, "-") {
		suffix = "-" + suffix
	}
	if len(baseName)+len(suffix) <= maxRunnerNameLen {
		return baseName + suffix
	}
	keep := maxRunnerNameLen - len(suffix)
	if keep < len(runnerNamePrefix)+1 {
		keep = len(runnerNamePrefix) + 1
	}
	if keep > len(baseName) {
		keep = len(baseName)
	}
	return strings.TrimRight(baseName[:keep], "-_.") + suffix
}

func sanitizeRunnerNamePart(value string) string {
	value = strings.TrimSpace(value)
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		allowed := (r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') ||
			r == '-' || r == '_' || r == '.'
		if allowed {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func isHexID(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if (r >= '0' && r <= '9') ||
			(r >= 'a' && r <= 'f') ||
			(r >= 'A' && r <= 'F') {
			continue
		}
		return false
	}
	return true
}

func runnerConfigured(runnerHome string) bool {
	if runnerHome == "" {
		runnerHome = "."
	}
	_, err := os.Stat(runnerHome + "/.runner")
	return err == nil
}

func cleanupRunnerState(runnerHome string) error {
	if runnerHome == "" {
		runnerHome = "."
	}
	var errs []string
	for _, rel := range []string{
		".runner",
		".credentials",
		".credentials_rsaparams",
		".env",
		".path",
		".service",
		"_work/_actions",
		"_work/_temp",
		"_work/_tool",
	} {
		if err := os.RemoveAll(filepath.Join(runnerHome, rel)); err != nil {
			errs = append(errs, rel+": "+err.Error())
		}
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

func ensureRunnerWorkspaces(ctx context.Context, cfg config) error {
	workRoot := filepath.Join(cfg.runnerHome, "_work")
	if err := os.MkdirAll(workRoot, 0o755); err != nil {
		return fmt.Errorf("create _work: %w", err)
	}
	repos, err := runnerWorkspaceRepos(ctx, cfg)
	if err != nil {
		return err
	}
	for _, repo := range repos {
		if err := os.MkdirAll(filepath.Join(workRoot, repo, repo), 0o755); err != nil {
			return fmt.Errorf("create workspace for %s: %w", repo, err)
		}
	}
	if len(repos) > 0 {
		log.Printf("precreated %d runner workspaces", len(repos))
	}
	return nil
}

func ensureRunnerJobHooks(runnerHome string) error {
	if runnerHome == "" {
		runnerHome = "."
	}
	hookPath := filepath.Join(runnerHome, "smoothnas-job-started-hook.sh")
	hook := `#!/bin/sh
set -eu

runner_home="${RUNNER_HOME:-/home/runner}"
repo="${GITHUB_REPOSITORY##*/}"

for major in 20 24; do
  dest="${runner_home}/externals/node${major}/bin/node"
  src=""
  for candidate in \
    "/usr/local/share/smoothnas-actions-node/node${major}/node" \
    "/opt/smoothnas/actions-node/node${major}/node"; do
    if [ -x "${candidate}" ]; then
      src="${candidate}"
      break
    fi
  done
  if [ ! -x "${dest}" ] && [ -n "${src}" ]; then
    mkdir -p "$(dirname "${dest}")"
    cp "${src}" "${dest}"
    chmod 755 "${dest}"
  fi
  if [ ! -x "${dest}" ]; then
    for chunk_dir in \
      "/usr/local/share/smoothnas-actions-node/node${major}" \
      "/opt/smoothnas/actions-node/node${major}"; do
      set -- "${chunk_dir}"/node.part.*
      if [ -f "$1" ]; then
        mkdir -p "$(dirname "${dest}")"
        cat "$@" > "${dest}.tmp"
        chmod 755 "${dest}.tmp"
        mv "${dest}.tmp" "${dest}"
        break
      fi
    done
  fi
done

if [ -n "${repo}" ]; then
  mkdir -p "${runner_home}/_work/${repo}/${repo}"
fi

if [ -n "${RUNNER_WORKSPACE:-}" ] && [ -n "${repo}" ]; then
  mkdir -p "${RUNNER_WORKSPACE}/${repo}"
fi
`
	if err := os.WriteFile(hookPath, []byte(hook), 0o755); err != nil {
		return fmt.Errorf("write job-started hook: %w", err)
	}
	if err := os.Setenv("ACTIONS_RUNNER_HOOK_JOB_STARTED", hookPath); err != nil {
		return fmt.Errorf("set job-started hook env: %w", err)
	}
	return nil
}

func runnerWorkspaceRepos(ctx context.Context, cfg config) ([]string, error) {
	if len(cfg.workspaceRepos) > 0 {
		return cleanRepoNames(cfg.workspaceRepos), nil
	}
	if !cfg.scope.IsOrg() {
		return []string{cfg.scope.repo}, nil
	}
	return listOrgRepoNames(ctx, http.DefaultClient, cfg.apiBase, cfg.scope, cfg.token)
}

func cleanRepoNames(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, repo := range in {
		repo = strings.TrimSpace(repo)
		if repo == "" || strings.Contains(repo, "/") || repo == "." || repo == ".." {
			continue
		}
		if seen[repo] {
			continue
		}
		seen[repo] = true
		out = append(out, repo)
	}
	return out
}

type dockerClient struct {
	base   string
	client *http.Client
}

type containerSummary struct {
	ID      string            `json:"Id"`
	Names   []string          `json:"Names"`
	Created int64             `json:"Created"`
	State   string            `json:"State"`
	Labels  map[string]string `json:"Labels"`
}

type containerInspect struct {
	ID     string `json:"Id"`
	Name   string `json:"Name"`
	Config struct {
		Image string `json:"Image"`
	} `json:"Config"`
	HostConfig inspectHostConfig `json:"HostConfig"`
	Mounts     []struct {
		Source      string `json:"Source"`
		Destination string `json:"Destination"`
	} `json:"Mounts"`
}

type inspectHostConfig struct {
	NetworkMode string `json:"NetworkMode"`
	NanoCPUs    int64  `json:"NanoCpus"`
	Memory      int64  `json:"Memory"`
	Binds       []string
}

type createContainerRequest struct {
	Image      string            `json:"Image"`
	Env        []string          `json:"Env,omitempty"`
	Labels     map[string]string `json:"Labels,omitempty"`
	HostConfig hostConfig        `json:"HostConfig"`
}

type hostConfig struct {
	Binds         []string      `json:"Binds,omitempty"`
	NetworkMode   string        `json:"NetworkMode,omitempty"`
	NanoCPUs      int64         `json:"NanoCpus,omitempty"`
	Memory        int64         `json:"Memory,omitempty"`
	RestartPolicy restartPolicy `json:"RestartPolicy"`
}

type restartPolicy struct {
	Name string `json:"Name"`
}

type createContainerResponse struct {
	ID string `json:"Id"`
}

func runController(ctx context.Context, cfg config) error {
	if cfg.workerCount < 1 {
		return fmt.Errorf("GH_RUNNER_WORKERS must be >= 1, got %d", cfg.workerCount)
	}
	if cfg.workerCPUs < 0 {
		return fmt.Errorf("GH_RUNNER_CPUS must be >= 0, got %g", cfg.workerCPUs)
	}
	if cfg.workerMemory < 0 {
		return fmt.Errorf("GH_RUNNER_MEMORY must be >= 0, got %d", cfg.workerMemory)
	}
	dc, err := newDockerClient(cfg.dockerHost)
	if err != nil {
		return err
	}

	selfID, _ := os.Hostname()
	self, err := dc.inspectContainer(ctx, selfID)
	if err != nil {
		return fmt.Errorf("inspect controller container %q: %w", selfID, err)
	}
	workspaceSource := ""
	if cfg.bindWorkspace {
		var err error
		workspaceSource, err = hostMountSource(self, filepath.Join(cfg.runnerHome, "_work"))
		if err != nil {
			return err
		}
	}
	dockerSocketSource, err := dockerSocketSource(self, "/var/run/docker.sock")
	if err != nil {
		return fmt.Errorf("find Docker socket mount: %w", err)
	}
	image := cfg.workerImage
	if image == "" {
		image = self.Config.Image
	}
	if image == "" {
		return errors.New("could not determine worker image; set GH_RUNNER_WORKER_IMAGE")
	}
	networkMode := self.HostConfig.NetworkMode
	workspaceMode := "ephemeral-rootfs"
	if cfg.bindWorkspace {
		workspaceMode = workspaceSource
	}
	log.Printf("starting controller: workers=%d image=%q network=%q workspace=%q", cfg.workerCount, image, networkMode, workspaceMode)

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	lastStaleSweep := time.Time{}
	for {
		if time.Since(lastStaleSweep) >= staleSweepEvery {
			runners, err := listGitHubRunners(ctx, http.DefaultClient, cfg.apiBase, cfg.scope, cfg.token)
			if err != nil {
				log.Printf("cleanup stale github runners: %v", err)
			} else {
				if err := removeOrphanedLocalWorkers(ctx, dc, cfg, runners, time.Now()); err != nil {
					log.Printf("cleanup orphaned local workers: %v", err)
				}
				stale := staleGitHubRunners(runners)
				if err := removeStaleLocalWorkers(ctx, dc, cfg, stale); err != nil {
					log.Printf("cleanup stale local workers: %v", err)
				}
				deleteStaleGitHubRunners(ctx, http.DefaultClient, cfg.apiBase, cfg.scope, cfg.token, stale)
				if err := removeMismatchedIdleWorkers(ctx, dc, cfg, image, runners); err != nil {
					log.Printf("replace stale idle workers: %v", err)
				}
				if err := shrinkExcessIdleWorkers(ctx, dc, cfg, runners); err != nil {
					log.Printf("shrink excess idle workers: %v", err)
				}
			}
			lastStaleSweep = time.Now()
		}
		if err := reconcileWorkers(ctx, dc, cfg, image, workspaceSource, dockerSocketSource, networkMode); err != nil {
			log.Printf("reconcile workers: %v", err)
		}
		select {
		case <-ctx.Done():
			return stopAllWorkers(context.Background(), dc, cfg)
		case <-ticker.C:
		}
	}
}

func reconcileWorkers(ctx context.Context, dc *dockerClient, cfg config, image, workspaceSource, dockerSocketSource, networkMode string) error {
	workers, err := dc.listWorkers(ctx)
	if err != nil {
		return err
	}
	running := 0
	for _, w := range workers {
		name := containerName(w)
		switch w.State {
		case "running":
			running++
		case "created":
			if err := dc.startContainer(ctx, w.ID); err != nil {
				log.Printf("start created worker %s: %v", name, err)
			} else {
				running++
			}
		default:
			log.Printf("removing completed worker %s state=%s", name, w.State)
			if err := dc.removeContainer(ctx, w.ID, true); err != nil {
				log.Printf("remove worker %s: %v", name, err)
			}
			removeWorkerHostWorkspace(cfg, name)
		}
	}
	for running < cfg.workerCount {
		if err := startWorker(ctx, dc, cfg, image, workspaceSource, dockerSocketSource, networkMode); err != nil {
			return err
		}
		running++
	}
	return nil
}

func shrinkExcessIdleWorkers(ctx context.Context, dc *dockerClient, cfg config, runners []githubRunner) error {
	workers, err := dc.listWorkers(ctx)
	if err != nil {
		return err
	}
	for _, w := range excessIdleWorkers(workers, runners, cfg.workerCount) {
		name := containerName(w)
		log.Printf("stopping excess idle worker %s", name)
		if err := dc.stopContainer(ctx, w.ID, 60); err != nil {
			log.Printf("stop excess idle worker %s: %v", name, err)
		}
		if err := dc.removeContainer(ctx, w.ID, true); err != nil {
			log.Printf("remove excess idle worker %s: %v", name, err)
			continue
		}
		removeWorkerHostWorkspace(cfg, name)
	}
	return nil
}

func excessIdleWorkers(workers []containerSummary, runners []githubRunner, target int) []containerSummary {
	running := 0
	for _, w := range workers {
		if w.State == "running" {
			running++
		}
	}
	if running <= target {
		return nil
	}
	excess := running - target
	var out []containerSummary
	for _, w := range workers {
		if len(out) >= excess {
			break
		}
		if w.State != "running" {
			continue
		}
		runner, ok := githubRunnerForWorker(runners, w.ID)
		if !ok || runner.Busy {
			continue
		}
		out = append(out, w)
	}
	return out
}

func removeMismatchedIdleWorkers(ctx context.Context, dc *dockerClient, cfg config, image string, runners []githubRunner) error {
	workers, err := dc.listWorkers(ctx)
	if err != nil {
		return err
	}
	for _, w := range workers {
		if w.State != "running" {
			continue
		}
		runner, ok := githubRunnerForWorker(runners, w.ID)
		if !ok || runner.Busy {
			continue
		}
		inspect, err := dc.inspectContainer(ctx, w.ID)
		if err != nil {
			log.Printf("inspect worker %s for resource drift: %v", containerName(w), err)
			continue
		}
		if workerSpecMatches(inspect, cfg, image) {
			continue
		}
		name := containerName(w)
		log.Printf("replacing idle worker %s with stale worker spec", name)
		if err := dc.stopContainer(ctx, w.ID, 60); err != nil {
			log.Printf("stop resource-mismatched worker %s: %v", name, err)
		}
		if err := dc.removeContainer(ctx, w.ID, true); err != nil {
			log.Printf("remove resource-mismatched worker %s: %v", name, err)
			continue
		}
		removeWorkerHostWorkspace(cfg, name)
	}
	return nil
}

func workerSpecMatches(inspect containerInspect, cfg config, image string) bool {
	if inspect.Config.Image != image {
		return false
	}
	return workerResourceLimitsMatch(inspect.HostConfig, cfg) &&
		hostConfigHasDestinationBind(inspect.HostConfig.Binds, "/var/run/docker.sock")
}

func workerResourceLimitsMatch(host inspectHostConfig, cfg config) bool {
	return host.NanoCPUs == workerNanoCPUs(cfg.workerCPUs) &&
		host.Memory == cfg.workerMemory
}

func hostConfigHasDestinationBind(binds []string, destination string) bool {
	for _, bind := range binds {
		parts := strings.Split(bind, ":")
		if len(parts) >= 2 && filepath.Clean(parts[1]) == filepath.Clean(destination) {
			return true
		}
	}
	return false
}

func startWorker(ctx context.Context, dc *dockerClient, cfg config, image, workspaceSource, dockerSocketSource, networkMode string) error {
	name := fmt.Sprintf("gh-runner-worker-%d", time.Now().UnixNano())
	binds := []string{dockerSocketSource + ":/var/run/docker.sock:rw"}
	if cfg.bindWorkspace {
		containerWorkspace := filepath.Join(cfg.runnerHome, "_work", "workers", name)
		if err := os.MkdirAll(containerWorkspace, 0o750); err != nil {
			return fmt.Errorf("create worker workspace: %w", err)
		}
		hostWorkspace := filepath.Join(workspaceSource, "workers", name)
		binds = append(binds, hostWorkspace+":"+filepath.Join(cfg.runnerHome, "_work")+":rw")
	}
	env := []string{
		"PATH=" + workerPath(),
		"COMPILER_PATH=" + workerCompilerPath(),
		"GH_RUNNER_MODE=worker",
		"GH_REPO_URL=" + cfg.repoURL,
		"GH_RUNNER_TOKEN=" + cfg.token,
		"GH_RUNNER_LABELS=" + cfg.labels,
		"GH_RUNNER_GROUP=" + cfg.group,
		"GH_API_BASE=" + cfg.apiBase,
		"GH_RUNNER_EPHEMERAL=true",
		"RUNNER_ALLOW_RUNASROOT=1",
		"RUNNER_HOME=" + cfg.runnerHome,
		"DOCKER_HOST=unix:///var/run/docker.sock",
		"DOTNET_SYSTEM_GLOBALIZATION_INVARIANT=1",
	}
	if len(cfg.dnsServers) > 0 {
		env = append(env, "GH_RUNNER_DNS_SERVERS="+strings.Join(cfg.dnsServers, ","))
	}
	if len(cfg.workspaceRepos) > 0 {
		env = append(env, "GH_RUNNER_WORKSPACE_REPOS="+strings.Join(cfg.workspaceRepos, ","))
	}
	req := createContainerRequest{
		Image:  image,
		Env:    env,
		Labels: map[string]string{workerLabelKey: "true"},
		HostConfig: hostConfig{
			Binds:         binds,
			NetworkMode:   networkMode,
			NanoCPUs:      workerNanoCPUs(cfg.workerCPUs),
			Memory:        cfg.workerMemory,
			RestartPolicy: restartPolicy{Name: "no"},
		},
	}
	id, err := dc.createContainer(ctx, name, req)
	if err != nil {
		removeWorkerHostWorkspace(cfg, name)
		return err
	}
	if err := dc.startContainer(ctx, id); err != nil {
		_ = dc.removeContainer(ctx, id, true)
		removeWorkerHostWorkspace(cfg, name)
		return err
	}
	log.Printf("started ephemeral worker %s (%s)", name, shortID(id))
	return nil
}

func workerPath() string {
	if value := os.Getenv("PATH"); value != "" {
		return value
	}
	return "/usr/local/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
}

func workerCompilerPath() string {
	if value := os.Getenv("COMPILER_PATH"); value != "" {
		return value
	}
	return "/usr/local/bin:/usr/libexec/gcc/x86_64-linux-gnu/14"
}

func stopAllWorkers(ctx context.Context, dc *dockerClient, cfg config) error {
	workers, err := dc.listWorkers(ctx)
	if err != nil {
		return err
	}
	for _, w := range workers {
		name := containerName(w)
		log.Printf("stopping worker %s", name)
		if w.State == "running" {
			_ = dc.stopContainer(ctx, w.ID, 60)
		}
		_ = dc.removeContainer(ctx, w.ID, true)
		removeWorkerHostWorkspace(cfg, name)
	}
	return nil
}

func removeStaleLocalWorkers(ctx context.Context, dc *dockerClient, cfg config, stale []githubRunner) error {
	if len(stale) == 0 {
		return nil
	}
	workers, err := dc.listWorkers(ctx)
	if err != nil {
		return err
	}
	for _, w := range workers {
		name := containerName(w)
		if !staleRunnerMatchesWorker(stale, w.ID) {
			continue
		}
		log.Printf("removing local worker %s for stale github runner", name)
		if w.State == "running" {
			_ = dc.stopContainer(ctx, w.ID, 30)
		}
		if err := dc.removeContainer(ctx, w.ID, true); err != nil {
			log.Printf("remove stale local worker %s: %v", name, err)
			continue
		}
		removeWorkerHostWorkspace(cfg, name)
	}
	return nil
}

func removeOrphanedLocalWorkers(ctx context.Context, dc *dockerClient, cfg config, runners []githubRunner, now time.Time) error {
	workers, err := dc.listWorkers(ctx)
	if err != nil {
		return err
	}
	for _, w := range workers {
		name := containerName(w)
		if !orphanedLocalWorker(w, runners, now) {
			continue
		}
		log.Printf("removing local worker %s with no github runner registration", name)
		if err := dc.stopContainer(ctx, w.ID, 30); err != nil {
			log.Printf("stop orphaned local worker %s: %v", name, err)
		}
		if err := dc.removeContainer(ctx, w.ID, true); err != nil {
			log.Printf("remove orphaned local worker %s: %v", name, err)
			continue
		}
		removeWorkerHostWorkspace(cfg, name)
	}
	return nil
}

func orphanedLocalWorker(w containerSummary, runners []githubRunner, now time.Time) bool {
	return w.State == "running" &&
		!workerHasGitHubRunner(runners, w.ID) &&
		workerRegistrationGraceExpired(w, now)
}

func staleRunnerMatchesWorker(stale []githubRunner, workerID string) bool {
	for _, runner := range stale {
		if runnerNameMatchesWorker(runner.Name, workerID) {
			return true
		}
	}
	return false
}

func workerHasGitHubRunner(runners []githubRunner, workerID string) bool {
	_, ok := githubRunnerForWorker(runners, workerID)
	return ok
}

func githubRunnerForWorker(runners []githubRunner, workerID string) (githubRunner, bool) {
	for _, runner := range runners {
		if runnerNameMatchesWorker(runner.Name, workerID) {
			return runner, true
		}
	}
	return githubRunner{}, false
}

func runnerNameMatchesWorker(runnerName, workerID string) bool {
	workerPrefix := runnerNamePrefix + shortID(workerID)
	return runnerName == workerPrefix || strings.HasPrefix(runnerName, workerPrefix+"-")
}

func workerRegistrationGraceExpired(w containerSummary, now time.Time) bool {
	return w.Created > 0 && now.Sub(time.Unix(w.Created, 0)) >= registrationGrace
}

func removeWorkerHostWorkspace(cfg config, name string) {
	if !cfg.bindWorkspace {
		return
	}
	_ = os.RemoveAll(filepath.Join(cfg.runnerHome, "_work", "workers", name))
}

func newDockerClient(host string) (*dockerClient, error) {
	u, err := url.Parse(host)
	if err != nil {
		return nil, fmt.Errorf("parse DOCKER_HOST: %w", err)
	}
	if u.Scheme != "unix" {
		return nil, fmt.Errorf("only unix:// DOCKER_HOST is supported, got %q", host)
	}
	socketPath := u.Path
	if socketPath == "" {
		return nil, fmt.Errorf("unix DOCKER_HOST missing socket path")
	}
	tr := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", socketPath)
		},
	}
	return &dockerClient{base: "http://docker", client: &http.Client{Transport: tr}}, nil
}

func (d *dockerClient) listWorkers(ctx context.Context) ([]containerSummary, error) {
	var out []containerSummary
	if err := d.do(ctx, http.MethodGet, "/containers/json?all=1", nil, &out); err != nil {
		return nil, err
	}
	workers := out[:0]
	for _, c := range out {
		if c.Labels[workerLabelKey] == "true" {
			workers = append(workers, c)
		}
	}
	return workers, nil
}

func (d *dockerClient) inspectContainer(ctx context.Context, id string) (containerInspect, error) {
	var out containerInspect
	err := d.do(ctx, http.MethodGet, "/containers/"+url.PathEscape(id)+"/json", nil, &out)
	return out, err
}

func (d *dockerClient) createContainer(ctx context.Context, name string, req createContainerRequest) (string, error) {
	var out createContainerResponse
	if err := d.do(ctx, http.MethodPost, "/containers/create?name="+url.QueryEscape(name), req, &out); err != nil {
		return "", err
	}
	if out.ID == "" {
		return "", errors.New("docker create response missing container id")
	}
	return out.ID, nil
}

func (d *dockerClient) startContainer(ctx context.Context, id string) error {
	return d.do(ctx, http.MethodPost, "/containers/"+url.PathEscape(id)+"/start", nil, nil)
}

func (d *dockerClient) stopContainer(ctx context.Context, id string, timeoutSeconds int) error {
	return d.do(ctx, http.MethodPost, fmt.Sprintf("/containers/%s/stop?t=%d", url.PathEscape(id), timeoutSeconds), nil, nil)
}

func (d *dockerClient) removeContainer(ctx context.Context, id string, force bool) error {
	return d.do(ctx, http.MethodDelete, fmt.Sprintf("/containers/%s?force=%t&v=1", url.PathEscape(id), force), nil, nil)
}

func (d *dockerClient) do(ctx context.Context, method, p string, in any, out any) error {
	var body io.Reader
	if in != nil {
		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(in); err != nil {
			return fmt.Errorf("encode docker request: %w", err)
		}
		body = &buf
	}
	req, err := http.NewRequestWithContext(ctx, method, d.base+p, body)
	if err != nil {
		return err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return fmt.Errorf("docker %s %s: %d %s", method, p, resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func hostMountSource(inspect containerInspect, destination string) (string, error) {
	for _, m := range inspect.Mounts {
		if filepath.Clean(m.Destination) == filepath.Clean(destination) && m.Source != "" {
			return m.Source, nil
		}
	}
	return "", fmt.Errorf("container has no host mount for %s", destination)
}

func dockerSocketSource(inspect containerInspect, destination string) (string, error) {
	source, err := hostMountSource(inspect, destination)
	if err == nil {
		return source, nil
	}
	if _, statErr := os.Stat(destination); statErr == nil {
		return hostRuntimeSocket, nil
	}
	if _, statErr := os.Stat(hostRuntimeSocket); statErr == nil {
		return hostRuntimeSocket, nil
	}
	return "", err
}

func containerName(c containerSummary) string {
	if len(c.Names) == 0 {
		return shortID(c.ID)
	}
	return strings.TrimPrefix(c.Names[0], "/")
}

func shortID(id string) string {
	if len(id) <= 12 {
		return id
	}
	return id[:12]
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
// GitHub API token, or "regtoken" otherwise. Detection is heuristic:
// GitHub registration tokens have no published prefix (they're opaque
// ~30-char base32-ish strings), but GitHub API tokens carry stable
// prefixes documented at
// https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/about-authentication-to-github#githubs-token-formats
func classifyToken(token string) string {
	switch {
	case strings.HasPrefix(token, "ghp_"),
		strings.HasPrefix(token, "github_pat_"),
		strings.HasPrefix(token, "gho_"),
		strings.HasPrefix(token, "ghu_"),
		strings.HasPrefix(token, "ghs_"): // server-to-server (GitHub App installation) tokens — also work for the registration-token endpoint
		return "pat"
	default:
		return "regtoken"
	}
}

// resolveRegistrationToken returns a usable runner registration
// token. For API tokens we POST to the registration-token endpoint to
// mint one; for direct registration tokens we pass the supplied value
// through unchanged.
func resolveRegistrationToken(ctx context.Context, client *http.Client, apiBase string, sc scope, token, kind string) (string, error) {
	if kind != "pat" {
		return token, nil
	}
	return mintRegistrationToken(ctx, client, apiBase, sc, token)
}

// mintRegistrationToken calls GitHub's runner registration-token
// endpoint, which mints a fresh ~1h registration token for the
// configured scope. The API token must have actions:write (repo scope)
// or admin:org scope (org scope).
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

func runnersEndpoint(apiBase string, sc scope) (string, error) {
	base, err := url.Parse(apiBase)
	if err != nil {
		return "", fmt.Errorf("parse api base: %w", err)
	}
	var p string
	if sc.IsOrg() {
		p = path.Join(base.Path, "orgs", sc.owner, "actions", "runners")
	} else {
		p = path.Join(base.Path, "repos", sc.owner, sc.repo, "actions", "runners")
	}
	out := *base
	out.Path = p
	return out.String(), nil
}

func orgReposEndpoint(apiBase string, sc scope) (string, error) {
	if !sc.IsOrg() {
		return "", errors.New("org repos endpoint requires org scope")
	}
	base, err := url.Parse(apiBase)
	if err != nil {
		return "", fmt.Errorf("parse api base: %w", err)
	}
	out := *base
	out.Path = path.Join(base.Path, "orgs", sc.owner, "repos")
	return out.String(), nil
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

type githubRunner struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
	Busy   bool   `json:"busy"`
}

type githubRepo struct {
	Name string `json:"name"`
}

func removeStaleGitHubRunners(ctx context.Context, client *http.Client, apiBase string, sc scope, pat string) error {
	stale, err := listStaleGitHubRunners(ctx, client, apiBase, sc, pat)
	if err != nil {
		return err
	}
	deleteStaleGitHubRunners(ctx, client, apiBase, sc, pat, stale)
	return nil
}

func listStaleGitHubRunners(ctx context.Context, client *http.Client, apiBase string, sc scope, pat string) ([]githubRunner, error) {
	runners, err := listGitHubRunners(ctx, client, apiBase, sc, pat)
	if err != nil {
		return nil, err
	}
	return staleGitHubRunners(runners), nil
}

func staleGitHubRunners(runners []githubRunner) []githubRunner {
	stale := make([]githubRunner, 0, len(runners))
	for _, runner := range runners {
		if staleSmoothNASRunner(runner) {
			stale = append(stale, runner)
		}
	}
	return stale
}

func deleteStaleGitHubRunners(ctx context.Context, client *http.Client, apiBase string, sc scope, pat string, stale []githubRunner) {
	for _, runner := range stale {
		log.Printf("removing stale github runner registration %q id=%d status=%s busy=%t", runner.Name, runner.ID, runner.Status, runner.Busy)
		if err := deleteGitHubRunner(ctx, client, apiBase, sc, pat, runner.ID); err != nil {
			log.Printf("delete stale github runner %q: %v", runner.Name, err)
		}
	}
}

func staleSmoothNASRunner(runner githubRunner) bool {
	return runner.ID > 0 &&
		strings.HasPrefix(runner.Name, runnerNamePrefix) &&
		strings.EqualFold(runner.Status, "offline")
}

func listGitHubRunners(ctx context.Context, client *http.Client, apiBase string, sc scope, pat string) ([]githubRunner, error) {
	endpoint, err := runnersEndpoint(apiBase, sc)
	if err != nil {
		return nil, err
	}
	out := []githubRunner{}
	for page := 1; ; page++ {
		u, err := url.Parse(endpoint)
		if err != nil {
			return nil, fmt.Errorf("parse runners endpoint: %w", err)
		}
		q := u.Query()
		q.Set("per_page", "100")
		q.Set("page", strconv.Itoa(page))
		u.RawQuery = q.Encode()

		var resp struct {
			Runners []githubRunner `json:"runners"`
		}
		if err := ghAPIJSON(ctx, client, http.MethodGet, u.String(), pat, nil, &resp); err != nil {
			return nil, err
		}
		out = append(out, resp.Runners...)
		if len(resp.Runners) < 100 {
			return out, nil
		}
	}
}

func listOrgRepoNames(ctx context.Context, client *http.Client, apiBase string, sc scope, pat string) ([]string, error) {
	endpoint, err := orgReposEndpoint(apiBase, sc)
	if err != nil {
		return nil, err
	}
	repos := []string{}
	seen := map[string]bool{}
	for page := 1; ; page++ {
		u, err := url.Parse(endpoint)
		if err != nil {
			return nil, fmt.Errorf("parse org repos endpoint: %w", err)
		}
		q := u.Query()
		q.Set("type", "all")
		q.Set("per_page", "100")
		q.Set("page", strconv.Itoa(page))
		u.RawQuery = q.Encode()

		var resp []githubRepo
		if err := ghAPIJSON(ctx, client, http.MethodGet, u.String(), pat, nil, &resp); err != nil {
			return nil, err
		}
		for _, repo := range resp {
			if repo.Name == "" || seen[repo.Name] {
				continue
			}
			seen[repo.Name] = true
			repos = append(repos, repo.Name)
		}
		if len(resp) < 100 {
			return repos, nil
		}
	}
}

func deleteGitHubRunner(ctx context.Context, client *http.Client, apiBase string, sc scope, pat string, id int64) error {
	endpoint, err := runnersEndpoint(apiBase, sc)
	if err != nil {
		return err
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("parse runners endpoint: %w", err)
	}
	u.Path = path.Join(u.Path, strconv.FormatInt(id, 10))
	if err := ghAPIJSON(ctx, client, http.MethodDelete, u.String(), pat, nil, nil); err != nil {
		return err
	}
	return nil
}

// ghAPIPostToken POSTs against a GitHub /actions/runners/*-token
// endpoint and returns the minted token field. Both registration
// and remove endpoints share the same response shape.
func ghAPIPostToken(ctx context.Context, client *http.Client, endpoint, pat string) (string, error) {
	var out struct {
		Token     string `json:"token"`
		ExpiresAt string `json:"expires_at"`
	}
	if err := ghAPIJSON(ctx, client, http.MethodPost, endpoint, pat, nil, &out); err != nil {
		return "", err
	}
	if out.Token == "" {
		return "", fmt.Errorf("github api response missing token field")
	}
	return out.Token, nil
}

func ghAPIJSON(ctx context.Context, client *http.Client, method, endpoint, pat string, in any, out any) error {
	var body io.Reader
	if in != nil {
		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(in); err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
		body = &buf
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+pat)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "smoothnas-plugin-gh-runner/0.1")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}
	if resp.StatusCode == http.StatusNotFound && method == http.MethodDelete {
		return nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("github api %s: %d %s", endpoint, resp.StatusCode, bytes.TrimSpace(respBody))
	}
	if out == nil || len(respBody) == 0 {
		return nil
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

// buildConfigArgs constructs the argv passed to ./config.sh. Org
// scope adds --runnergroup; repo scope ignores it (GitHub rejects
// the flag at repo level).
func buildConfigArgs(repoURL, regToken, labels, group, name string, isOrg, ephemeral bool) []string {
	args := []string{
		"--url", repoURL,
		"--token", regToken,
		"--labels", labels,
		"--name", name,
		"--unattended",
		"--replace",
	}
	if ephemeral {
		args = append(args, "--ephemeral")
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
// run.sh exiting on its own. For API tokens we mint a fresh remove
// token and call config.sh remove; for direct registration tokens we
// can't (the API endpoint requires an API token and the registration token
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

func envList(k, def string) []string {
	raw := envOr(k, def)
	var out []string
	for _, part := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n'
	}) {
		if v := strings.TrimSpace(part); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func envBool(k string, def bool) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(k)))
	if v == "" {
		return def
	}
	switch v {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		log.Printf("invalid boolean %s=%q; using default %v", k, os.Getenv(k), def)
		return def
	}
}

func envInt(k string, def int) int {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		log.Printf("invalid integer %s=%q; using default %d", k, v, def)
		return def
	}
	return n
}

func envFloat(k string, def float64) float64 {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def
	}
	n, err := strconv.ParseFloat(v, 64)
	if err != nil {
		log.Printf("invalid float %s=%q; using default %g", k, v, def)
		return def
	}
	return n
}

func envBytes(k string, def int64) int64 {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def
	}
	n, err := parseByteSize(v)
	if err != nil {
		log.Printf("invalid byte size %s=%q: %v; using default %d", k, v, err, def)
		return def
	}
	return n
}

func workerNanoCPUs(cpus float64) int64 {
	if cpus <= 0 {
		return 0
	}
	return int64(cpus * 1_000_000_000)
}

func parseByteSize(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, errors.New("empty size")
	}
	i := 0
	for i < len(value) {
		c := value[i]
		if (c >= '0' && c <= '9') || c == '.' {
			i++
			continue
		}
		break
	}
	if i == 0 {
		return 0, fmt.Errorf("missing numeric value")
	}
	n, err := strconv.ParseFloat(value[:i], 64)
	if err != nil {
		return 0, err
	}
	if n < 0 {
		return 0, fmt.Errorf("must be non-negative")
	}
	unit := strings.ToLower(strings.TrimSpace(value[i:]))
	multiplier := float64(1)
	switch unit {
	case "", "b":
	case "k", "kb", "kib":
		multiplier = 1 << 10
	case "m", "mb", "mib":
		multiplier = 1 << 20
	case "g", "gb", "gib":
		multiplier = 1 << 30
	case "t", "tb", "tib":
		multiplier = 1 << 40
	default:
		return 0, fmt.Errorf("unsupported unit %q", unit)
	}
	return int64(n * multiplier), nil
}

func sleepOrDone(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
