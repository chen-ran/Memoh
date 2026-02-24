#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
TARGET_OS="${TARGET_OS:-$(go env GOOS)}"
TARGET_ARCH="${TARGET_ARCH:-$(go env GOARCH)}"
BUN_VERSION="${BUN_VERSION:-latest}"
VERSION="${VERSION:-dev}"
COMMIT_HASH="${COMMIT_HASH:-unknown}"
BUILD_TIME="${BUILD_TIME:-$(date -u +"%Y-%m-%dT%H:%M:%SZ")}"
OUTPUT_DIR="${OUTPUT_DIR:-$ROOT_DIR/dist}"
PREPARE_ASSETS_ONLY="false"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --os)
      TARGET_OS="$2"
      shift 2
      ;;
    --arch)
      TARGET_ARCH="$2"
      shift 2
      ;;
    --bun-version)
      BUN_VERSION="$2"
      shift 2
      ;;
    --version)
      VERSION="$2"
      shift 2
      ;;
    --commit-hash)
      COMMIT_HASH="$2"
      shift 2
      ;;
    --output-dir)
      OUTPUT_DIR="$2"
      shift 2
      ;;
    --prepare-assets)
      PREPARE_ASSETS_ONLY="true"
      shift
      ;;
    *)
      echo "Unknown arg: $1" >&2
      exit 1
      ;;
  esac
done

prepare_assets() {
  local web_dir="$ROOT_DIR/internal/embedded/web"
  local agent_dir="$ROOT_DIR/internal/embedded/agent"
  local bun_dir="$ROOT_DIR/internal/embedded/bun/${TARGET_OS}-${TARGET_ARCH}"

  rm -rf "$web_dir" "$agent_dir" "$bun_dir"
  mkdir -p "$web_dir" "$agent_dir" "$bun_dir"

  echo "[release] building web assets"
  pnpm --dir "$ROOT_DIR" web:build
  cp -R "$ROOT_DIR/packages/web/dist/." "$web_dir/"

  echo "[release] building agent bundle"
  pnpm --dir "$ROOT_DIR" agent:build
  mkdir -p "$agent_dir/dist"
  cp "$ROOT_DIR/agent/dist/index.js" "$agent_dir/dist/index.js"
  if [[ -f "$ROOT_DIR/agent/package.json" ]]; then
    cp "$ROOT_DIR/agent/package.json" "$agent_dir/package.json"
  fi

  local bun_target=""
  case "${TARGET_OS}-${TARGET_ARCH}" in
    linux-amd64) bun_target="bun-linux-x64.zip" ;;
    linux-arm64) bun_target="bun-linux-aarch64.zip" ;;
    darwin-amd64) bun_target="bun-darwin-x64.zip" ;;
    darwin-arm64) bun_target="bun-darwin-aarch64.zip" ;;
    windows-amd64) bun_target="bun-windows-x64.zip" ;;
    windows-arm64) bun_target="bun-windows-aarch64.zip" ;;
    *)
      echo "bun runtime not available for ${TARGET_OS}-${TARGET_ARCH}" > "$bun_dir/UNAVAILABLE"
      echo "[release] skipped bun bundle for unsupported target ${TARGET_OS}-${TARGET_ARCH}"
      return 0
      ;;
  esac

  local tmp_dir
  tmp_dir="$(mktemp -d)"
  trap 'rm -rf "$tmp_dir"' RETURN

  local url
  if [[ "$BUN_VERSION" == "latest" ]]; then
    url="https://github.com/oven-sh/bun/releases/latest/download/${bun_target}"
  else
    url="https://github.com/oven-sh/bun/releases/download/bun-v${BUN_VERSION}/${bun_target}"
  fi

  echo "[release] downloading ${url}"
  curl -fsSL "$url" -o "$tmp_dir/bun.zip"
  unzip -q -o "$tmp_dir/bun.zip" -d "$tmp_dir"

  local bun_bin_name="bun"
  if [[ "$TARGET_OS" == "windows" ]]; then
    bun_bin_name="bun.exe"
  fi

  local bun_source_path=""
  if [[ -f "$tmp_dir/${bun_target%.zip}/${bun_bin_name}" ]]; then
    bun_source_path="$tmp_dir/${bun_target%.zip}/${bun_bin_name}"
  else
    for candidate in "$tmp_dir"/bun-"${TARGET_OS}"-*/"${bun_bin_name}"; do
      if [[ -f "$candidate" ]]; then
        bun_source_path="$candidate"
        break
      fi
    done
  fi

  if [[ -z "$bun_source_path" ]]; then
    echo "failed to locate bun binary in downloaded archive" >&2
    exit 1
  fi

  cp "$bun_source_path" "$bun_dir/$bun_bin_name"
  chmod +x "$bun_dir/$bun_bin_name" || true

  echo "[release] embedded assets prepared (${TARGET_OS}-${TARGET_ARCH})"
}

build_archive() {
  mkdir -p "$OUTPUT_DIR"

  local ext=""
  if [[ "$TARGET_OS" == "windows" ]]; then
    ext=".exe"
  fi

  local binary_name="memoh${ext}"
  local target_dir="$OUTPUT_DIR/memoh_${VERSION}_${TARGET_OS}_${TARGET_ARCH}"
  mkdir -p "$target_dir"

  echo "[release] building binary ${TARGET_OS}/${TARGET_ARCH}"
  CGO_ENABLED=0 GOOS="$TARGET_OS" GOARCH="$TARGET_ARCH" \
    go build \
    -trimpath \
    -ldflags "-s -w -X github.com/memohai/memoh/internal/version.Version=${VERSION} -X github.com/memohai/memoh/internal/version.CommitHash=${COMMIT_HASH} -X github.com/memohai/memoh/internal/version.BuildTime=${BUILD_TIME}" \
    -o "$target_dir/$binary_name" \
    "$ROOT_DIR/cmd/memoh"

  if [[ "$TARGET_OS" == "windows" ]]; then
    (cd "$OUTPUT_DIR" && zip -q -r "memoh_${VERSION}_${TARGET_OS}_${TARGET_ARCH}.zip" "memoh_${VERSION}_${TARGET_OS}_${TARGET_ARCH}")
  else
    tar -C "$OUTPUT_DIR" -czf "$OUTPUT_DIR/memoh_${VERSION}_${TARGET_OS}_${TARGET_ARCH}.tar.gz" "memoh_${VERSION}_${TARGET_OS}_${TARGET_ARCH}"
  fi

  echo "[release] archive created (${TARGET_OS}-${TARGET_ARCH})"
}

prepare_assets
if [[ "$PREPARE_ASSETS_ONLY" == "true" ]]; then
  echo "[release] prepare-assets only mode completed"
  exit 0
fi

build_archive
