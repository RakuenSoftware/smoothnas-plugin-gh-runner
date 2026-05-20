#!/usr/bin/env bash
set -euo pipefail

image="${1:?usage: check-image-runtimes.sh IMAGE}"

docker run --rm --entrypoint /bin/sh "$image" -c 'test "$( . /etc/os-release && printf %s "$ID:$VERSION_ID" )" = debian:13'
docker run --rm --entrypoint /home/runner/externals/node20/bin/node "$image" --version
docker run --rm --entrypoint /home/runner/externals/node24/bin/node "$image" --version

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

docker save "$image" -o "$tmp/image.tar"
tar -tf "$tmp/image.tar" | grep '/layer.tar$' > "$tmp/layers"

for major in 20 24; do
  found_node=0
  found_whiteout=0
  layer_index=0

  while IFS= read -r layer; do
    entries="$tmp/layer-${major}-${layer_index}.txt"
    tar -xOf "$tmp/image.tar" "$layer" | tar -tf - > "$entries"

    if grep -Eq "(^\\./)?home/runner/externals/node${major}/bin/node$" "$entries"; then
      found_node=1
    fi
    if grep -Eq "(^\\./)?home/runner/externals/node${major}/bin/\\.wh\\.node$" "$entries"; then
      found_whiteout=1
    fi

    layer_index=$((layer_index + 1))
  done < "$tmp/layers"

  if [ "$found_node" -ne 1 ]; then
    echo "node${major}/bin/node is missing from saved image layers" >&2
    exit 1
  fi
  if [ "$found_whiteout" -ne 0 ]; then
    echo "node${major}/bin/node has an OCI whiteout in saved image layers" >&2
    exit 1
  fi
done
