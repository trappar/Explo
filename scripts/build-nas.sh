#!/bin/sh
set -eu
cd "$(git rev-parse --show-toplevel)"
if [ -n "$(git status --porcelain)" ]; then
  echo "Commit the source before building a deployable image." >&2; exit 1
fi
nas_host=${NAS_HOST:-nas}
revision=$(git rev-parse HEAD)
short_revision=$(git rev-parse --short=12 HEAD)
version="trappar-$short_revision"
image="navistack/explo:git-$short_revision"
remote_dir="/volume1/docker/explo/releases/$revision"
archive=$(mktemp)
trap 'rm -f "$archive"' EXIT HUP INT TERM
git archive --format=tar.gz HEAD > "$archive"
ssh "$nas_host" "mkdir -p '$remote_dir'"
scp -O "$archive" "$nas_host:$remote_dir/source.tar.gz"
ssh "$nas_host" "set -eu
cd '$remote_dir'
tar -xzf source.tar.gz
rm source.tar.gz
/usr/local/bin/docker build \
  --build-arg BUILDPLATFORM=linux/amd64 \
  --build-arg TARGETARCH=amd64 \
  --build-arg VERSION='$version' \
  --label org.opencontainers.image.source=https://github.com/trappar/Explo \
  --label org.opencontainers.image.revision='$revision' \
  --label org.opencontainers.image.version='$version' \
  -t '$image' ."
cat <<EOF
Built $image. Select these settings for the explo service in the NAS Compose project:
    image: $image
    build:
      context: ./releases/$revision
      dockerfile: Dockerfile
      args:
        BUILDPLATFORM: linux/amd64
        TARGETARCH: amd64
        VERSION: $version
No running container was changed. Deploy when the current playlist job is idle.
EOF
