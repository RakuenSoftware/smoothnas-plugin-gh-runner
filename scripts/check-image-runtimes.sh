#!/usr/bin/env bash
set -euo pipefail

image="${1:?usage: check-image-runtimes.sh IMAGE}"

docker run --rm --entrypoint /bin/sh "$image" -c 'test "$( . /etc/os-release && printf %s "$ID:$VERSION_ID" )" = debian:13'
docker run --rm --entrypoint /home/runner/externals/node20/bin/node "$image" --version
docker run --rm --entrypoint /home/runner/externals/node24/bin/node "$image" --version
docker run --rm --entrypoint /bin/sh "$image" -c 'test ! -L /home/runner/externals/node20/bin/node'
docker run --rm --entrypoint /bin/sh "$image" -c 'test ! -L /home/runner/externals/node24/bin/node'

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

docker save "$image" -o "$tmp/image.tar"
python3 - "$tmp/image.tar" <<'PY'
import gzip
import io
import json
import sys
import tarfile

archive = sys.argv[1]
wanted = {
    "20": {
        "node": "home/runner/externals/node20/bin/node",
        "whiteout": "home/runner/externals/node20/bin/.wh.node",
    },
    "24": {
        "node": "home/runner/externals/node24/bin/node",
        "whiteout": "home/runner/externals/node24/bin/.wh.node",
    },
}
seen = {major: {"node": False, "node_symlink": False, "whiteout": False} for major in wanted}


def clean(name):
    return name.lstrip("./")


def scan_layer(blob):
    try:
        layer = tarfile.open(fileobj=io.BytesIO(blob), mode="r:*")
    except tarfile.ReadError:
        try:
            layer = tarfile.open(fileobj=io.BytesIO(gzip.decompress(blob)), mode="r:")
        except (OSError, tarfile.ReadError):
            return
    with layer:
        for member in layer:
            name = clean(member.name)
            for major, paths in wanted.items():
                if name == paths["node"]:
                    seen[major]["node"] = True
                    if member.issym():
                        seen[major]["node_symlink"] = True
                if name == paths["whiteout"]:
                    seen[major]["whiteout"] = True


with tarfile.open(archive, mode="r") as image:
    names = set(image.getnames())
    layer_names = [name for name in names if name.endswith("/layer.tar")]

    if "manifest.json" in names:
        manifest = json.load(image.extractfile("manifest.json"))
        for item in manifest:
            for name in item.get("Layers", []):
                if name in names:
                    scan_layer(image.extractfile(name).read())
    elif "index.json" in names:
        index = json.load(image.extractfile("index.json"))
        for item in index.get("manifests", []):
            digest = item.get("digest", "")
            algo, _, value = digest.partition(":")
            manifest_name = f"blobs/{algo}/{value}"
            if not value or manifest_name not in names:
                continue
            manifest = json.load(image.extractfile(manifest_name))
            for layer in manifest.get("layers", []):
                digest = layer.get("digest", "")
                algo, _, value = digest.partition(":")
                layer_name = f"blobs/{algo}/{value}"
                if value and layer_name in names:
                    scan_layer(image.extractfile(layer_name).read())
    else:
        for name in layer_names:
            scan_layer(image.extractfile(name).read())

failed = False
for major, state in seen.items():
    if not state["node"]:
        print(f"node{major}/bin/node is missing from saved image layers", file=sys.stderr)
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
