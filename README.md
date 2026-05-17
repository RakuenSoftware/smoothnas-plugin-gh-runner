# smoothnas-plugin-gh-runner

The GitHub Actions self-hosted runner reference plugin for SmoothNAS. The plugin runs one controller container that starts disposable one-job runner containers against the configured repo or organisation.

This is the second reference plugin built against the [SmoothNAS plugin system](https://github.com/RakuenSoftware/smoothnas/blob/main/docs/proposals/pending/smoothnas-plugins.md). It exercises three things [llama.cpp](https://github.com/RakuenSoftware/smoothnas-plugin-llama-cpp) doesn't:

1. **Ephemeral workers** — operators commonly need several runners for queue throughput, but long-lived runner registrations and workspaces get stale. The controller keeps a configured number of one-shot worker containers available and replaces each worker after it handles one job.
2. **No SmoothNAS UI to embed** — GitHub.com is the UI. `ui.embed` is omitted entirely.
3. **Outbound-only** workload — no inbound `ports`, no nginx route. The bridge network's NAT egress is enough.

## Operator workflow

In the SmoothNAS UI:

1. **Get a token from GitHub.** Either:
   - Create a fine-grained PAT with `actions:write` scope on the target repo, use a GitHub App installation token, or use an OAuth token from `gh auth token` with sufficient access. Paste it as `GH_RUNNER_TOKEN`. Short-lived registration tokens from the GitHub UI are not supported in controller mode because each worker needs a fresh registration token.
2. **Install** → paste this manifest into the wizard, set `GH_REPO_URL` (e.g. `https://github.com/my-org/my-repo`) and `GH_RUNNER_TOKEN`, pick a tier with SSD slot capacity, and choose `GH_RUNNER_WORKERS` for concurrency.
3. **Start** → click Start on the plugin card; tierd materialises the controller container. The controller uses the SmoothNAS runtime socket to start worker containers. Each worker registers with GitHub as ephemeral, appears in the runner list while idle or running, handles one job, then exits and is removed with its workspace.
4. **Use** → target the runners from a workflow:
   ```yaml
   jobs:
     build:
       runs-on: [self-hosted, smoothnas]
   ```
5. **Scale** → change `GH_RUNNER_WORKERS` and restart the plugin. The controller reconciles the worker pool to that count.

Uninstall via the UI's Danger Zone stops the controller. The controller stops/removes workers, workers deregister through the SIGTERM path, and SmoothNAS removes the plugin image and workspace volume.

## Controller and Worker Modes

The actions/runner tarball ships with `config.sh` (registration) and `run.sh` (the runner loop) but no glue that fits SmoothNAS' lifecycle. The `wrapper/` Go binary has two modes:

- `GH_RUNNER_MODE=controller` (default): inspect the controller container, discover the host path behind `/home/runner/_work`, and use the SmoothNAS runtime socket to create/remove worker containers.
- `GH_RUNNER_MODE=worker`: register one ephemeral GitHub runner, run exactly one job, clean local runner/action/tool state, and exit.

Controller mode requires SmoothNAS' `runtime-control` plugin profile. That profile mounts `/run/smoothnas-runtime/docker.sock` at `/var/run/docker.sock` and sets `DOCKER_HOST=unix:///var/run/docker.sock`; it intentionally does not grant Wolf's device or capability set.

The worker path:

1. Reads `GH_REPO_URL` / `GH_RUNNER_TOKEN` / `GH_RUNNER_LABELS` / `GH_RUNNER_GROUP` from env.
2. Detects whether the supplied token is a GitHub API token (`ghp_`, `github_pat_`, `gho_`, `ghu_`, or `ghs_`) or already a registration token.
3. For API tokens, calls the GitHub API:
   - Repo URL → `POST /repos/{owner}/{repo}/actions/runners/registration-token`
   - Org URL → `POST /orgs/{org}/actions/runners/registration-token`
4. Runs `./config.sh --url $GH_REPO_URL --token $REG_TOKEN --labels $GH_RUNNER_LABELS --runnergroup $GH_RUNNER_GROUP --name "smoothnas-${HOSTNAME}-..." --unattended --replace --ephemeral`.
5. Traps `SIGTERM` / `SIGINT`; on receipt, fetches a removal token with the API credential and runs `./config.sh remove --token $REM_TOKEN` before exiting.
6. Runs `./run.sh`; when the one job completes, the runner exits and the worker container is removed by the controller.

The wrapper code is Go with unit tests covering the token-type heuristic, repo-vs-org URL parsing, Docker runtime helpers, and the GitHub API call shape against an `httptest` server.

## Worker Workspaces

Each worker gets a fresh workspace directory under the controller's tier-bound volume:

```
/mnt/<tier>/.plugins/gh-runner/workspace/workers/<worker-name>/  →  /home/runner/_work
```

Workspaces are not shared and are deleted after the worker exits. That avoids stale `_actions`, `_temp`, `_tool`, and repository checkout state across jobs.

## Local development

```sh
# wrapper smoke build + tests
cd wrapper && go vet ./... && go test -race ./...

# image build
docker buildx build -t smoothnas-plugin-gh-runner:dev .

# run a single registration locally (won't actually register without a real token)
docker run --rm \
  -e GH_RUNNER_MODE=worker \
  -e GH_REPO_URL=https://github.com/owner/repo \
  -e GH_RUNNER_TOKEN=ghp_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx \
  smoothnas-plugin-gh-runner:dev
```

To sideload a dev image into a SmoothNAS dev VM, edit `smoothnas-plugin.yaml`'s `artifact.image` to your local tag and use `tierd-cli plugin install`.

## Release flow

`.github/workflows/release.yml` runs on tag push (`v*`):

1. Runs `go vet` + `go test -race` against the wrapper.
2. Builds and pushes a single image to `ghcr.io/rakuensoftware/smoothnas-plugin-gh-runner:VER`.
3. Resolves the pushed digest and rewrites `artifact.digest` in the manifest.
4. Creates a GitHub release attaching the rewritten manifest.

The smoke test that actually installs the plugin against a SmoothNAS dev VM lives in the SmoothNAS repo's CI, not here. Triggered nightly: spins up SmoothNAS, sideloads the manifest with `count: 1`, supplies a CI-secret PAT against a private test repo, asserts the runner picks up a one-line workflow within 5 minutes, then uninstalls and verifies the runner deregisters.

## Pinning the actions/runner version

The Dockerfile pins `RUNNER_VERSION` and `RUNNER_SHA256` as build-args. To upgrade:

1. Find the desired version on [actions/runner releases](https://github.com/actions/runner/releases).
2. `curl -fsSLO https://github.com/actions/runner/releases/download/vX.Y.Z/actions-runner-linux-amd64-X.Y.Z.tar.gz && sha256sum *.tar.gz`
3. Bump `RUNNER_VERSION` and `RUNNER_SHA256` in `Dockerfile`.
4. Tag a new release; CI rebuilds and republishes.

## License

Add a LICENSE file at publish time. The wrapper code in this repo is original; the actions/runner downloaded into the image carries its own license terms (MIT).
