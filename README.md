# smoothnas-plugin-gh-runner

The GitHub Actions self-hosted runner reference plugin for SmoothNAS. Each instance registers itself with a configured repo or organisation on container start and deregisters cleanly on stop.

This is the second reference plugin built against the [SmoothNAS plugin system](https://github.com/RakuenSoftware/smoothnas/blob/main/docs/proposals/pending/smoothnas-plugins.md). It exercises three things [llama.cpp](https://github.com/RakuenSoftware/smoothnas-plugin-llama-cpp) doesn't:

1. **Multiple instances** of the same plugin — operators commonly run several runners in parallel for queue throughput. The manifest declares `instances.count: 2, configurable: true`.
2. **No SmoothNAS UI to embed** — GitHub.com is the UI. `ui.embed` is omitted entirely.
3. **Outbound-only** workload — no inbound `ports`, no nginx route. The bridge network's NAT egress is enough.

## Operator workflow

In the SmoothNAS UI:

1. **Get a token from GitHub.** Either:
   - **PAT (recommended).** Create a fine-grained PAT with `actions:write` scope on the target repo (or `admin:org` for org runners). Paste it as `GH_RUNNER_TOKEN`. The wrapper will exchange the PAT for a registration token on every container start, so restarts don't need operator intervention.
   - **Registration token.** From the repo's *Settings → Actions → Runners → New self-hosted runner* page, copy the short-lived token. Paste as `GH_RUNNER_TOKEN`. Each restart needs a fresh paste because these expire in ~1 hour.
2. **Install** → paste this manifest into the wizard, set `GH_REPO_URL` (e.g. `https://github.com/my-org/my-repo`) and `GH_RUNNER_TOKEN`, pick a tier with SSD slot capacity, install. Default `count: 2` creates two runner containers.
3. **Start** → click Start on the plugin card; tierd materialises both containers, the wrapper registers each with GitHub, and they appear in the repo's runner list with `smoothnas` in their labels.
4. **Use** → target the runners from a workflow:
   ```yaml
   jobs:
     build:
       runs-on: [self-hosted, smoothnas]
   ```
5. **Scale** → on the plugin's detail page, the Instances tab exposes a slider that POSTs to `/api/plugins/gh-runner/instances`. Scaling down stops + deregisters the top-numbered instances; scaling up materialises additional containers.

Uninstall via the UI's Danger Zone stops all instances, deregisters them via the SIGTERM trap, removes the containers and per-instance workspace directories, and removes the cached image. Workspaces are deleted along with everything else (per parent doc all-or-none policy).

## Why a wrapper image?

The actions/runner tarball ships with `config.sh` (registration) and `run.sh` (the runner loop) but no glue that fits SmoothNAS' lifecycle. The `wrapper/` Go binary fills that gap:

1. Reads `GH_REPO_URL` / `GH_RUNNER_TOKEN` / `GH_RUNNER_LABELS` / `GH_RUNNER_GROUP` from env.
2. Detects whether the supplied token is a PAT (prefix `ghp_` / `github_pat_`) or already a registration token.
3. For PATs, calls the GitHub API:
   - Repo URL → `POST /repos/{owner}/{repo}/actions/runners/registration-token`
   - Org URL → `POST /orgs/{org}/actions/runners/registration-token`
4. Runs `./config.sh --url $GH_REPO_URL --token $REG_TOKEN --labels $GH_RUNNER_LABELS --runnergroup $GH_RUNNER_GROUP --name "smoothnas-${HOSTNAME}" --unattended --replace`.
5. Traps `SIGTERM` / `SIGINT`; on receipt, fetches a removal token (PATs only) and runs `./config.sh remove --token $REM_TOKEN` before exiting, so an uninstall deregisters the runner from GitHub cleanly. Registration-token mode skips remove (the token is single-use); the operator cleans stale entries from the GitHub UI.
6. Otherwise, exec's `./run.sh`.

The wrapper code is ~250 LoC of Go with unit tests covering the token-type heuristic, repo-vs-org URL parsing, and the API call shape against an `httptest` server.

## Multiple instances

Each runner instance gets its own workspace directory bind-mounted from the tier-bound volume:

```
/mnt/<tier>/.plugins/gh-runner/instance-1/workspace/  →  /home/runner/_work
/mnt/<tier>/.plugins/gh-runner/instance-2/workspace/  →  /home/runner/_work
```

Workspaces are not shared — concurrent jobs on different runners can each cache `node_modules` independently without stomping each other. The `volumes[].perInstance: true` flag in the manifest is what makes this happen.

The `instances` block (`count` + `configurable`) and the `perInstance` volumes flag are documented in the plugin system's parent proposal and have been in the schema since phase 01. This plugin is the first published manifest where they matter in practice.

## Local development

```sh
# wrapper smoke build + tests
cd wrapper && go vet ./... && go test -race ./...

# image build
docker buildx build -t smoothnas-plugin-gh-runner:dev .

# run a single registration locally (won't actually register without a real token)
docker run --rm \
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
