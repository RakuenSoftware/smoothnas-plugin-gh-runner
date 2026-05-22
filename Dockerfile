# SmoothNAS plugin: GitHub Actions ephemeral runner controller and
# one-shot worker image.
#
# Built FROM debian:13-slim — no upstream "official" actions/runner
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

FROM golang:1.25-bookworm AS go-runtime

FROM golang:1.25-alpine AS tools-build
RUN go install github.com/google/go-containerregistry/cmd/crane@v0.20.6

FROM docker:28-cli AS docker-cli

# --- final image ---
FROM debian:13-slim

# RUNNER_VERSION is overridable by CI to track upstream releases. The
# matching tarball SHA is pinned via RUNNER_SHA256 so the image build
# fails loudly if upstream rotates a tag.
ARG RUNNER_VERSION=2.334.0
ARG RUNNER_SHA256_X64=048024cd2c848eb6f14d5646d56c13a4def2ae7ee3ad12122bee960c56f3d271
ARG RUNNER_SHA256_ARM64=f44255bd3e80160eb25f71bc83d06ea025f6908748807a584687b3184759f7e4
ARG RUNNER_SHA256_ARM=84a25196caf971d0c634e32864731e773e1668235f799666fc0ec40ac666a0ab
ARG NODE20_VERSION=20.20.2
ARG NODE20_SHA256_X64=df770b2a6f130ed8627c9782c988fda9669fa23898329a61a871e32f965e007d
ARG NODE20_SHA256_ARM64=73093db209e4e9e09dd7d15a47aeaab1b74833830df03efa5f942a1122c5fa71
ARG NODE20_SHA256_ARM=f704ce75d9a194c30c378049b516000e49612c2f046ac83c7435eb33ec2926f0
ARG NODE24_VERSION=24.15.0
ARG NODE24_SHA256_X64=472655581fb851559730c48763e0c9d3bc25975c59d518003fc0849d3e4ba0f6
ARG NODE24_SHA256_ARM64=f3d5a797b5d210ce8e2cb265544c8e482eaedcb8aa409a8b46da7e8595d0dda0
ARG LLAMA_REF=24cabf4d08d460cfb6e73fa308a15b34e2b04600
ARG LLAMA_ARCHIVE_SHA256=e3cde0d7b955d6f96a363b41d865f67ceefb736ccdc74f7a2793ca35600fb5f6
ARG TARGETARCH=amd64

ENV DEBIAN_FRONTEND=noninteractive
ENV RUNNER_ALLOW_RUNASROOT=1
ENV DOTNET_SYSTEM_GLOBALIZATION_INVARIANT=1
ENV PATH=/usr/local/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
ENV COMPILER_PATH=/usr/local/bin:/usr/libexec/gcc/x86_64-linux-gnu/14

# Runtime deps the actions runner needs (curl/jq for our wrapper's
# GitHub + runtime API calls; git/ca-certs/tar/sudo because the runner expects
# them; libicu for the .NET-based runner host). Self-hosted workflow jobs also
# expect the hosted-runner basics: gh for release dispatch and docker-cli for
# build/push against the mounted SmoothNAS runtime socket.
#
# Bake the CUDA/Vulkan llama-cpp release toolchain into the runner image so
# accelerator jobs do not install packages after a worker starts.
RUN set -eux; \
    apt-get update; \
    apt-get install -y --no-install-recommends ca-certificates curl; \
    printf '%s\n' 'deb [trusted=yes] https://developer.download.nvidia.com/compute/cuda/repos/ubuntu2404/x86_64/ /' \
      > /etc/apt/sources.list.d/cuda-ubuntu2404-x86_64.list; \
    apt-get update; \
    apt-get install -y --no-install-recommends \
        ca-certificates \
        curl \
        docker-cli \
        git \
        gh \
        jq \
        libicu76 \
        nodejs \
        sudo \
        tar \
        xz-utils \
        binutils \
        build-essential \
        cmake \
        gcc-14 \
        g++-14 \
        libgomp1 \
        libssl-dev \
        cuda-nvcc-12-8 \
        cuda-nvvm-12-8 \
        cuda-cudart-dev-12-8 \
        cuda-driver-dev-12-8 \
        glslc \
        libegl1 \
        libgl1 \
        libgles2 \
        libglvnd0 \
        libglx0 \
        libvulkan-dev \
        libvulkan1 \
        libxcb-cursor-dev \
        libxcb-xinerama0 \
        libxcb-xinput0 \
        mesa-vulkan-drivers \
        spirv-headers; \
    install -m 0755 /usr/bin/docker /usr/local/bin/docker; \
    /usr/local/bin/docker --version; \
    /usr/local/cuda/bin/nvcc --version; \
    glslc --version; \
    cmake --version; \
    cuda_math=/usr/local/cuda/targets/x86_64-linux/include/crt/math_functions.h; \
    sed -i -E \
      -e 's/(extern __DEVICE_FUNCTIONS_DECL__ __device_builtin__ double[[:space:]]+rsqrt\(double x\));/\1 noexcept (true);/' \
      -e 's/(extern __DEVICE_FUNCTIONS_DECL__ __device_builtin__ float[[:space:]]+rsqrtf\(float x\));/\1 noexcept (true);/' \
      -e 's/(extern __DEVICE_FUNCTIONS_DECL__ __device_builtin__ double[[:space:]]+sinpi\(double x\));/\1 noexcept (true);/' \
      -e 's/(extern __DEVICE_FUNCTIONS_DECL__ __device_builtin__ float[[:space:]]+sinpif\(float x\));/\1 noexcept (true);/' \
      -e 's/(extern __DEVICE_FUNCTIONS_DECL__ __device_builtin__ double[[:space:]]+cospi\(double x\));/\1 noexcept (true);/' \
      -e 's/(extern __DEVICE_FUNCTIONS_DECL__ __device_builtin__ float[[:space:]]+cospif\(float x\));/\1 noexcept (true);/' \
      -e 's/__func__\(double rsqrt\(double a\)\);/__func__(double rsqrt(double a) noexcept (true));/' \
      -e 's/__func__\(float rsqrtf\(float a\)\);/__func__(float rsqrtf(float a) noexcept (true));/' \
      -e 's/__func__\(double sinpi\(double a\)\);/__func__(double sinpi(double a) noexcept (true));/' \
      -e 's/__func__\(float sinpif\(float a\)\);/__func__(float sinpif(float a) noexcept (true));/' \
      -e 's/__func__\(double cospi\(double a\)\);/__func__(double cospi(double a) noexcept (true));/' \
      -e 's/__func__\(float cospif\(float a\)\);/__func__(float cospif(float a) noexcept (true));/' \
      "$cuda_math"; \
    cc1_path="$(dpkg -L cpp-14-x86-64-linux-gnu gcc-14-x86-64-linux-gnu | grep '/cc1$' | head -n1)"; \
    cc1plus_path="$(dpkg -L g++-14-x86-64-linux-gnu | grep '/cc1plus$' | head -n1)"; \
    test -x "$cc1_path"; \
    test -x "$cc1plus_path"; \
    install -m 0755 "$cc1_path" /usr/local/bin/cc1; \
    install -m 0755 "$cc1plus_path" /usr/local/bin/cc1plus; \
    printf 'int main() { return 0; }\n' > /tmp/c-sanity.c; \
    printf 'int main() { return 0; }\n' > /tmp/cxx-sanity.cpp; \
    gcc-14 /tmp/c-sanity.c -o /tmp/c-sanity; \
    g++-14 /tmp/cxx-sanity.cpp -o /tmp/cxx-sanity; \
    printf '#include <math.h>\n__global__ void k() {}\nint main() { k<<<1,1>>>(); return 0; }\n' > /tmp/cuda-sanity.cu; \
    /usr/local/cuda/bin/nvcc -c /tmp/cuda-sanity.cu -o /tmp/cuda-sanity.o; \
    rm -f /tmp/c-sanity.c /tmp/c-sanity /tmp/cxx-sanity.cpp /tmp/cxx-sanity /tmp/cuda-sanity.cu /tmp/cuda-sanity.o; \
    rm -rf /var/lib/apt/lists/*

COPY --from=go-runtime /usr/local/go /usr/local/go
COPY --from=tools-build /go/bin/crane /usr/local/bin/crane
COPY --from=docker-cli /usr/local/bin/docker /usr/local/bin/docker
RUN go version && crane version && docker --version

# Bake the Atomic llama.cpp TurboQuant/MTP source used by the llama-cpp
# plugin release workflow. Worker jobs build from this local tree instead of
# cloning or downloading source at job runtime.
RUN set -eux; \
    curl -fsSLo /tmp/llama.tar.gz \
      "https://github.com/AtomicBot-ai/atomic-llama-cpp-turboquant/archive/${LLAMA_REF}.tar.gz"; \
    echo "${LLAMA_ARCHIVE_SHA256}  /tmp/llama.tar.gz" | sha256sum -c -; \
    mkdir -p /opt/atomic-llama-cpp-turboquant; \
    tar -xzf /tmp/llama.tar.gz --strip-components=1 -C /opt/atomic-llama-cpp-turboquant; \
    printf '%s\n' "${LLAMA_REF}" > /opt/atomic-llama-cpp-turboquant/.smoothnas-llama-ref; \
    rm /tmp/llama.tar.gz; \
    test -f /opt/atomic-llama-cpp-turboquant/CMakeLists.txt

# Non-root runner user. Matches what GitHub's official install
# instructions recommend; the runner refuses to start as root by
# default.
RUN useradd -m -d /home/runner -s /bin/bash runner \
 && mkdir -p /home/runner/_work \
 && chown -R runner:runner /home/runner

USER runner
WORKDIR /home/runner

# Pull and verify the actions/runner tarball, then install Node runtime
# binaries in the runner's externals tree. JavaScript actions execute these
# exact paths; keep real files here because SmoothNAS' LXC rootfs import does
# not preserve symlink targets outside the runner tree consistently.
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
 && rm runner.tar.gz; \
    install_node() { \
      major="$1"; \
      version="$2"; \
      arch="$3"; \
      sha256="$4"; \
      url="https://nodejs.org/dist/v${version}/node-v${version}-linux-${arch}.tar.xz"; \
      curl -fsSLo node.tar.xz "$url"; \
      echo "${sha256}  node.tar.xz" | sha256sum -c -; \
      tar -xJf node.tar.xz; \
      node_bin="/home/runner/externals/node${major}/bin/node"; \
      mkdir -p "$(dirname "$node_bin")"; \
      cat "node-v${version}-linux-${arch}/bin/node" > "$node_bin"; \
      chmod 0755 "$node_bin"; \
      rm -rf node.tar.xz "node-v${version}-linux-${arch}"; \
      "/home/runner/externals/node${major}/bin/node" --version; \
    }; \
    case "${TARGETARCH}" in \
      amd64) node_arch="x64"; node20_sha="${NODE20_SHA256_X64}"; node24_sha="${NODE24_SHA256_X64}" ;; \
      arm64) node_arch="arm64"; node20_sha="${NODE20_SHA256_ARM64}"; node24_sha="${NODE24_SHA256_ARM64}" ;; \
      arm) node_arch="armv7l"; node20_sha="${NODE20_SHA256_ARM}"; node24_sha="" ;; \
      *) echo "unsupported TARGETARCH=${TARGETARCH}" >&2; exit 1 ;; \
    esac; \
    install_node 20 "${NODE20_VERSION}" "$node_arch" "$node20_sha"; \
    if [ -n "$node24_sha" ]; then \
      install_node 24 "${NODE24_VERSION}" "$node_arch" "$node24_sha"; \
    else \
      echo "Node.js 24 does not publish linux-${node_arch} binaries" >&2; \
      exit 1; \
    fi

# Runner's bundled dependency installer needs root.
USER root
RUN /home/runner/bin/installdependencies.sh \
 && for major in 20 24; do \
      test -x "/home/runner/externals/node${major}/bin/node"; \
      install -D -m 0755 \
        "/home/runner/externals/node${major}/bin/node" \
        "/usr/local/share/smoothnas-actions-node/node${major}/node"; \
      install -D -m 0755 \
        "/home/runner/externals/node${major}/bin/node" \
        "/opt/smoothnas/actions-node/node${major}/node"; \
      split -b 8m -d -a 3 \
        "/home/runner/externals/node${major}/bin/node" \
        "/usr/local/share/smoothnas-actions-node/node${major}/node.part."; \
      split -b 8m -d -a 3 \
        "/home/runner/externals/node${major}/bin/node" \
        "/opt/smoothnas/actions-node/node${major}/node.part."; \
      chmod 0644 \
        /usr/local/share/smoothnas-actions-node/node${major}/node.part.* \
        /opt/smoothnas/actions-node/node${major}/node.part.*; \
      "/home/runner/externals/node${major}/bin/node" --version; \
    done \
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
