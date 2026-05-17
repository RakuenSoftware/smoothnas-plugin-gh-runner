# SmoothNAS plugin: GitHub Actions ephemeral runner controller and
# one-shot worker image.
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
ARG RUNNER_VERSION=2.334.0
ARG RUNNER_SHA256_X64=048024cd2c848eb6f14d5646d56c13a4def2ae7ee3ad12122bee960c56f3d271
ARG RUNNER_SHA256_ARM64=f44255bd3e80160eb25f71bc83d06ea025f6908748807a584687b3184759f7e4
ARG RUNNER_SHA256_ARM=84a25196caf971d0c634e32864731e773e1668235f799666fc0ec40ac666a0ab
ARG TARGETARCH=amd64

ENV DEBIAN_FRONTEND=noninteractive
ENV RUNNER_ALLOW_RUNASROOT=1

# Runtime deps the actions runner needs (curl/jq for our wrapper's
# GitHub + runtime API calls; git/ca-certs/tar/sudo because the runner expects
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
RUN set -eux; \
    case "${TARGETARCH}" in \
      amd64) runner_arch="x64"; sha256="${RUNNER_SHA256_X64}" ;; \
      arm64) runner_arch="arm64"; sha256="${RUNNER_SHA256_ARM64}" ;; \
      arm) runner_arch="arm"; sha256="${RUNNER_SHA256_ARM}" ;; \
      *) echo "unsupported TARGETARCH=${TARGETARCH}" >&2; exit 1 ;; \
    esac; \
    curl -fsSLo runner.tar.gz \
        "https://github.com/actions/runner/releases/download/v${RUNNER_VERSION}/actions-runner-linux-${runner_arch}-${RUNNER_VERSION}.tar.gz"; \
    echo "${sha256}  runner.tar.gz" | sha256sum -c - \
 && tar xzf runner.tar.gz \
 && rm runner.tar.gz

# Runner's bundled dependency installer needs root.
USER root
RUN /home/runner/bin/installdependencies.sh \
 && rm -rf /var/lib/apt/lists/*

COPY --from=wrapper-build /smoothnas-wrapper /usr/local/bin/smoothnas-wrapper

# SmoothNAS creates plugin bind-mount directories as root. Run the
# wrapper as root so the controller workspace and optional worker
# workspace binds are writable inside LXC; RUNNER_ALLOW_RUNASROOT
# above permits the actions runner to operate in this appliance runtime.
USER root
WORKDIR /home/runner

# The wrapper defaults to controller mode. Worker containers set
# GH_RUNNER_MODE=worker and use the same image to register an
# ephemeral one-job GitHub Actions runner.
ENTRYPOINT ["/usr/local/bin/smoothnas-wrapper"]
