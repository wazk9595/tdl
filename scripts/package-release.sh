#!/usr/bin/env bash
set -euo pipefail

version=${1:?version required}
target_os=${2:?target OS required}
target_arch=${3:?target architecture required}
output_dir=${4:-dist}

[[ "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+-video\.[0-9]+$ ]] || { echo 'Invalid release tag' >&2; exit 1; }
case "$target_os" in
  windows) os_name=Windows; binary=tdl.exe; extension=zip ;;
  linux) os_name=Linux; binary=tdl; extension=tar.gz ;;
  darwin) os_name=MacOS; binary=tdl; extension=tar.gz ;;
  *) echo 'Unsupported OS' >&2; exit 1 ;;
esac
case "$target_arch" in
  amd64) arch_name=64bit ;;
  arm64) arch_name=arm64 ;;
  *) echo 'Unsupported architecture' >&2; exit 1 ;;
esac

mkdir -p "$output_dir"
output_dir=$(cd "$output_dir" && pwd)
stage=$(mktemp -d "$output_dir/stage.XXXXXX")
commit=$(git rev-parse --short=12 HEAD)
commit_date=$(git show -s --format=%cI HEAD)
ldflags="-s -w -X github.com/iyear/tdl/pkg/consts.Version=$version -X github.com/iyear/tdl/pkg/consts.Commit=$commit -X github.com/iyear/tdl/pkg/consts.CommitDate=$commit_date"
CGO_ENABLED=0 GOOS="$target_os" GOARCH="$target_arch" go build -trimpath -ldflags "$ldflags" -o "$stage/$binary" .
cp LICENSE README.md README_zh.md INSTALL-video.md "$stage/"
if [[ "$target_os" != windows ]]; then chmod 755 "$stage/$binary"; fi

if [[ "$target_os" == "$(go env GOHOSTOS)" && "$target_arch" == "$(go env GOHOSTARCH)" ]]; then
  "$stage/$binary" version
  "$stage/$binary" upload --help | grep -q -- --video-mode
  "$stage/$binary" send --help | grep -q -- --allow-transcode
  "$stage/$binary" forward --help | grep -q -- --exclude
fi

archive="$output_dir/tdl_${os_name}_${arch_name}.${extension}"
if [[ "$extension" == zip ]]; then
  (cd "$stage" && zip -q -9 "$archive" "$binary" LICENSE README.md README_zh.md INSTALL-video.md)
else
  tar -czf "$archive" -C "$stage" "$binary" LICENSE README.md README_zh.md INSTALL-video.md
fi
echo "$archive"
