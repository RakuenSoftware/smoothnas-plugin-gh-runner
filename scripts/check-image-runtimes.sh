#!/usr/bin/env bash
set -euo pipefail

image="${1:?usage: check-image-runtimes.sh IMAGE}"

docker run --rm --entrypoint /bin/sh "$image" -c 'test "$( . /etc/os-release && printf %s "$ID:$VERSION_ID" )" = debian:13'
docker run --rm --entrypoint /home/runner/externals/node20/bin/node "$image" --version
docker run --rm --entrypoint /home/runner/externals/node24/bin/node "$image" --version
docker run --rm --entrypoint /bin/sh "$image" -c 'test ! -L /home/runner/externals/node20/bin/node'
docker run --rm --entrypoint /bin/sh "$image" -c 'test ! -L /home/runner/externals/node24/bin/node'
docker run --rm --entrypoint /bin/sh "$image" -c 'test -x /usr/local/share/smoothnas-actions-node/node20/node'
docker run --rm --entrypoint /bin/sh "$image" -c 'test -x /usr/local/share/smoothnas-actions-node/node24/node'
docker run --rm --entrypoint /bin/sh "$image" -c 'test -x /opt/smoothnas/actions-node/node20/node'
docker run --rm --entrypoint /bin/sh "$image" -c 'test -x /opt/smoothnas/actions-node/node24/node'
docker run --rm --entrypoint /bin/sh "$image" -c 'test -f /usr/local/share/smoothnas-actions-node/node20/node.part.000'
docker run --rm --entrypoint /bin/sh "$image" -c 'test -f /usr/local/share/smoothnas-actions-node/node24/node.part.000'
docker run --rm --entrypoint /bin/sh "$image" -c 'test -f /opt/smoothnas/actions-node/node20/node.part.000'
docker run --rm --entrypoint /bin/sh "$image" -c 'test -f /opt/smoothnas/actions-node/node24/node.part.000'
docker run --rm --entrypoint /bin/sh "$image" -c 'go version'
docker run --rm --entrypoint /bin/sh "$image" -c 'crane version'
docker run --rm --entrypoint /bin/sh "$image" -c 'docker --version'
docker run --rm --entrypoint /bin/sh "$image" -c '/usr/local/cuda/bin/nvcc --version'
docker run --rm --entrypoint /bin/sh "$image" -c 'test -x /usr/local/cuda/bin/nvcc'
docker run --rm --entrypoint /bin/sh "$image" -c 'test -x /usr/local/cuda/bin/ptxas'
docker run --rm --entrypoint /bin/sh "$image" -c 'test -x /usr/local/cuda/bin/nvlink'
docker run --rm --entrypoint /bin/sh "$image" -c 'test -x /usr/local/bin/cicc'
docker run --rm --entrypoint /bin/sh "$image" -c 'test -f /usr/local/cuda/targets/x86_64-linux/lib/libcublas.so'
docker run --rm --entrypoint /bin/sh "$image" -c 'test -f /usr/local/cuda/targets/x86_64-linux/lib/libcublasLt.so'
docker run --rm --entrypoint /bin/sh "$image" -c 'test -f /usr/local/share/cuda-nvvm/libdevice/libdevice.10.bc'
docker run --rm --entrypoint /bin/sh "$image" -c 'test -f /usr/local/share/smoothnas-toolchain/nvcc/nvcc.part.000'
docker run --rm --entrypoint /bin/sh "$image" -c 'test -f /usr/local/share/smoothnas-toolchain/ptxas/ptxas.part.000'
docker run --rm --entrypoint /bin/sh "$image" -c 'test -f /usr/local/share/smoothnas-toolchain/nvlink/nvlink.part.000'
docker run --rm --entrypoint /bin/sh "$image" -c 'test -f /usr/local/share/smoothnas-toolchain/cc1/cc1.part.000'
docker run --rm --entrypoint /bin/sh "$image" -c 'test -f /usr/local/share/smoothnas-toolchain/cc1plus/cc1plus.part.000'
docker run --rm --entrypoint /bin/sh "$image" -c 'test -f /usr/local/share/smoothnas-toolchain/cicc/cicc.part.000'
docker run --rm --entrypoint /bin/sh "$image" -c 'test -f /usr/local/share/smoothnas-toolchain/libdevice.10.bc/libdevice.10.bc.part.000'
docker run --rm --entrypoint /bin/sh "$image" -c 'test -f /usr/local/share/smoothnas-toolchain/libcublas.so/libcublas.so.part.000'
docker run --rm --entrypoint /bin/sh "$image" -c 'test -f /usr/local/share/smoothnas-toolchain/libcublasLt.so/libcublasLt.so.part.000'
docker run --rm --entrypoint /bin/sh "$image" -c 'grep -q "^CICC_PATH[[:space:]]*= /usr/local/bin$" /usr/local/cuda/bin/nvcc.profile'
docker run --rm --entrypoint /bin/sh "$image" -c 'grep -q "^NVVMIR_LIBRARY_DIR[[:space:]]*= /usr/local/share/cuda-nvvm/libdevice$" /usr/local/cuda/bin/nvcc.profile'
docker run --rm --entrypoint /bin/sh "$image" -c 'printf "#include <math.h>\n__global__ void k() {}\nint main() { k<<<1,1>>>(); return 0; }\n" >/tmp/cuda-sanity.cu && /usr/local/cuda/bin/nvcc -c /tmp/cuda-sanity.cu -o /tmp/cuda-sanity.o'
docker run --rm --entrypoint /bin/sh "$image" -c 'glslc --version'
docker run --rm --entrypoint /bin/sh "$image" -c 'cmake --version'
docker run --rm --entrypoint /bin/sh "$image" -c 'g++-14 --version'
docker run --rm --entrypoint /bin/sh "$image" -c 'test -x /usr/local/bin/cc1 && test -x /usr/local/bin/cc1plus'
docker run --rm --entrypoint /bin/sh "$image" -c 'echo "int main() { return 0; }" >/tmp/c-sanity.c && gcc-14 /tmp/c-sanity.c -o /tmp/c-sanity'
docker run --rm --entrypoint /bin/sh "$image" -c 'echo "int main() { return 0; }" >/tmp/cxx-sanity.cpp && g++-14 /tmp/cxx-sanity.cpp -o /tmp/cxx-sanity'
docker run --rm --entrypoint /bin/sh "$image" -c 'test -f /opt/atomic-llama-cpp-turboquant/CMakeLists.txt'
docker run --rm --entrypoint /bin/sh "$image" -c 'test "$(cat /opt/atomic-llama-cpp-turboquant/.smoothnas-llama-ref)" = 0a635dcd92ba66c75fccfef91c3e106f4668f367'
docker run --rm --entrypoint /bin/sh "$image" -c 'dpkg-query -W gcc-14 g++-14 cmake cuda-nvcc-12-8 cuda-cudart-dev-12-8 libcublas-dev-12-8 libvulkan-dev glslc'

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

cat > "$tmp/scan-image-layers.py" <<'PY'
import sys
import tarfile

wanted = {
    "20": {
        "node": "home/runner/externals/node20/bin/node",
        "backup": "usr/local/share/smoothnas-actions-node/node20/node",
        "opt_backup": "opt/smoothnas/actions-node/node20/node",
        "chunk": "usr/local/share/smoothnas-actions-node/node20/node.part.000",
        "opt_chunk": "opt/smoothnas/actions-node/node20/node.part.000",
        "whiteout": "home/runner/externals/node20/bin/.wh.node",
    },
    "24": {
        "node": "home/runner/externals/node24/bin/node",
        "backup": "usr/local/share/smoothnas-actions-node/node24/node",
        "opt_backup": "opt/smoothnas/actions-node/node24/node",
        "chunk": "usr/local/share/smoothnas-actions-node/node24/node.part.000",
        "opt_chunk": "opt/smoothnas/actions-node/node24/node.part.000",
        "whiteout": "home/runner/externals/node24/bin/.wh.node",
    },
}
seen = {major: {"node": False, "backup": False, "opt_backup": False, "chunk": False, "opt_chunk": False, "node_symlink": False, "whiteout": False} for major in wanted}


def clean(name):
    return name.lstrip("./")


def drain(fileobj):
    while fileobj.read(1024 * 1024):
        pass


def scan_layer_file(fileobj):
    try:
        layer = tarfile.open(fileobj=fileobj, mode="r|*")
    except tarfile.ReadError:
        drain(fileobj)
        return
    with layer:
        for member in layer:
            name = clean(member.name)
            for major, paths in wanted.items():
                if name == paths["node"]:
                    seen[major]["node"] = True
                    if member.issym():
                        seen[major]["node_symlink"] = True
                if name == paths["backup"]:
                    seen[major]["backup"] = True
                if name == paths["opt_backup"]:
                    seen[major]["opt_backup"] = True
                if name == paths["chunk"]:
                    seen[major]["chunk"] = True
                if name == paths["opt_chunk"]:
                    seen[major]["opt_chunk"] = True
                if name == paths["whiteout"]:
                    seen[major]["whiteout"] = True
    drain(fileobj)


with tarfile.open(fileobj=sys.stdin.buffer, mode="r|*") as image:
    for member in image:
        name = clean(member.name)
        if not name.endswith("/layer.tar") and not name.startswith("blobs/"):
            continue
        extracted = image.extractfile(member)
        if extracted is None:
            continue
        scan_layer_file(extracted)

failed = False
for major, state in seen.items():
    if not state["node"]:
        print(f"node{major}/bin/node is missing from saved image layers", file=sys.stderr)
        failed = True
    if not state["backup"]:
        print(f"node{major} /usr/local/share backup is missing from saved image layers", file=sys.stderr)
        failed = True
    if not state["opt_backup"]:
        print(f"node{major} /opt backup is missing from saved image layers", file=sys.stderr)
        failed = True
    if not state["chunk"]:
        print(f"node{major} /usr/local/share chunked backup is missing from saved image layers", file=sys.stderr)
        failed = True
    if not state["opt_chunk"]:
        print(f"node{major} /opt chunked backup is missing from saved image layers", file=sys.stderr)
        failed = True
    if state["node_symlink"]:
        print(f"node{major}/bin/node must be a real file, not a symlink", file=sys.stderr)
        failed = True
    if state["whiteout"]:
        print(f"node{major}/bin/node has an OCI whiteout in saved image layers", file=sys.stderr)
        failed = True

if failed:
    sys.exit(1)
PY

docker save "$image" | python3 "$tmp/scan-image-layers.py"
