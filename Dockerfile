# SmoothNAS plugin: GitHub Actions self-hosted runner with
# registration-shim wrapper.
#
# Built FROM ubuntu:22.04 — no upstream "official" actions/runner
# image exists, so we install the runner tarball ourselves at known
# pinned versions. The wrapper is a small Go binary that handles
# registration token exchange, config.sh, SIGTERM-driven graceful
# deregistration, and exec into run.sh.

# --- wrapper build ---
FROM golang:1.25-alpine AS wrapper-build
WORKDIR /src
COPY wrapper/go.mod wrapper/main.go ./
# CGO off + static link so the binary runs on the slim runtime base
# regardless of glibc/musl differences.
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /smoothnas-wrapper .

# --- final image ---
FROM ubuntu:22.04

# RUNNER_VERSION is overridable by CI to track upstream releases. The
# matching tarball SHA is pinned via RUNNER_SHA256 so the image build
# fails loudly if upstream rotates a tag.
ARG RUNNER_VERSION=2.321.0
ARG RUNNER_SHA256=ba46ba7ce3a4d7236b16fbe44419fb453bc08f866b24f04d549ec89f1722a29e
ARG TARGETARCH=amd64

ENV DEBIAN_FRONTEND=noninteractive

# Runtime deps the actions runner needs (curl/jq for our wrapper's
# GitHub API calls; git/ca-certs/tar/sudo because the runner expects
# them; libicu for the .NET-based runner host).
RUN apt-get update \
 && apt-get install -y --no-install-recommends \
        ca-certificates \
        curl \
        git \
        jq \
        libicu70 \
        sudo \
        tar \
 && rm -rf /var/lib/apt/lists/*

# Non-root runner user. Matches what GitHub's official install
# instructions recommend; the runner refuses to start as root by
# default.
RUN useradd -m -d /home/runner -s /bin/bash runner \
 && mkdir -p /home/runner/_work \
 && chown -R runner:runner /home/runner

USER runner
WORKDIR /home/runner

# Pull and verify the actions/runner tarball.
RUN curl -fsSLo runner.tar.gz \
        "https://github.com/actions/runner/releases/download/v${RUNNER_VERSION}/actions-runner-linux-${TARGETARCH}-${RUNNER_VERSION}.tar.gz" \
 && echo "${RUNNER_SHA256}  runner.tar.gz" | sha256sum -c - \
 && tar xzf runner.tar.gz \
 && rm runner.tar.gz

# Runner's bundled dependency installer needs root.
USER root
RUN /home/runner/bin/installdependencies.sh \
 && rm -rf /var/lib/apt/lists/*

COPY --from=wrapper-build /smoothnas-wrapper /usr/local/bin/smoothnas-wrapper

USER runner
WORKDIR /home/runner

# The wrapper handles registration, run.sh exec, and SIGTERM
# deregistration. SmoothNAS injects GH_REPO_URL / GH_RUNNER_TOKEN /
# GH_RUNNER_LABELS / GH_RUNNER_GROUP into the container env from the
# manifest's config block.
ENTRYPOINT ["/usr/local/bin/smoothnas-wrapper"]
