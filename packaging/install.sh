#!/usr/bin/env bash
# Install Ctrl+Shift+3 CLI (cs3) to ~/bin and start it.
# Usage: curl -fsSL https://ctrlshift3.com/CLI/install.sh | bash
set -euo pipefail

BASE_URL="${CS3_BASE_URL:-https://ctrlshift3.com}"
BASE_URL="${BASE_URL%/}"

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch="$(uname -m)"

case "$os" in
  darwin)
    case "$arch" in
      arm64|aarch64) zip_name="cs3-darwin-arm64.zip" ;;
      x86_64) zip_name="cs3-darwin-amd64.zip" ;;
      *)
        echo "Unsupported Mac architecture: $arch" >&2
        exit 1
        ;;
    esac
    ;;
  linux)
    case "$arch" in
      x86_64|amd64) zip_name="cs3-linux-amd64.zip" ;;
      *)
        echo "Unsupported Linux architecture: $arch (need amd64)." >&2
        exit 1
        ;;
    esac
    ;;
  mingw*|msys*|cygwin*|windows*)
    echo "On Windows, open PowerShell and run:" >&2
    echo "  irm ${BASE_URL}/CLI/install.ps1 | iex" >&2
    exit 1
    ;;
  *)
    echo "Unsupported OS: $(uname -s)" >&2
    exit 1
    ;;
esac

if ! command -v curl >/dev/null 2>&1; then
  echo "curl is required." >&2
  exit 1
fi
if ! command -v unzip >/dev/null 2>&1; then
  echo "unzip is required." >&2
  exit 1
fi

dest_dir="${HOME}/bin"
dest="${dest_dir}/cs3"
url="${BASE_URL}/CLI/${zip_name}"
sums_url="${BASE_URL}/CLI/SHA256SUMS"

tmpdir="$(mktemp -d "${TMPDIR:-/tmp}/cs3-install.XXXXXX")"
cleanup() { rm -rf "$tmpdir"; }
trap cleanup EXIT

# Fetch checksums first, then download the zip with ?h=<hash> so CDNs
# (e.g. Cloudflare) cannot serve a stale zip against a newer SHA256SUMS.
expected=""
if curl -fsSL "$sums_url" -o "${tmpdir}/SHA256SUMS"; then
  expected="$(awk -v f="$zip_name" '$2 == f { print $1; exit }' "${tmpdir}/SHA256SUMS")"
  if [[ -z "$expected" ]]; then
    echo "Checksum list has no entry for ${zip_name}." >&2
    exit 1
  fi
  url="${url}?h=${expected}"
else
  echo "Warning: could not download SHA256SUMS; continuing without verify." >&2
fi

echo "Downloading ${url} …"
curl -fsSL "$url" -o "${tmpdir}/cs3.zip"

if [[ -n "$expected" ]]; then
  if command -v shasum >/dev/null 2>&1; then
    actual="$(shasum -a 256 "${tmpdir}/cs3.zip" | awk '{print $1}')"
  elif command -v sha256sum >/dev/null 2>&1; then
    actual="$(sha256sum "${tmpdir}/cs3.zip" | awk '{print $1}')"
  else
    echo "Warning: no shasum/sha256sum; skipping checksum verify." >&2
    actual=""
  fi
  if [[ -n "$actual" ]]; then
    if [[ "$actual" != "$expected" ]]; then
      echo "Checksum mismatch for ${zip_name}." >&2
      echo "Expected: ${expected}" >&2
      echo "Got:      ${actual}" >&2
      echo "If you just redeployed, purge the CDN cache for /CLI/ and retry." >&2
      exit 1
    fi
    echo "Checksum OK."
  fi
fi

unzip -qo "${tmpdir}/cs3.zip" -d "$tmpdir"
if [[ ! -f "${tmpdir}/cs3" ]]; then
  echo "Zip did not contain cs3." >&2
  exit 1
fi

mkdir -p "$dest_dir"
install -m 755 "${tmpdir}/cs3" "$dest"

if [[ "$os" == "darwin" ]]; then
  xattr -dr com.apple.quarantine "$dest" 2>/dev/null || true
fi

echo "Installed to ${dest}"

case ":${PATH}:" in
  *":${dest_dir}:"*) ;;
  *)
    echo
    echo "Note: ${dest_dir} is not on your PATH."
    if [[ -n "${ZSH_VERSION:-}" ]] || [[ "${SHELL:-}" == *zsh* ]]; then
      echo "Add it with:  echo 'export PATH=\"\$HOME/bin:\$PATH\"' >> ~/.zshrc && source ~/.zshrc"
    else
      echo "Add it with:  echo 'export PATH=\"\$HOME/bin:\$PATH\"' >> ~/.bashrc && source ~/.bashrc"
    fi
    ;;
esac

echo "Starting cs3 …"
echo
# curl|bash leaves stdin as the script pipe (EOF). Attach the real terminal.
if [[ -r /dev/tty ]]; then
  exec "$dest" </dev/tty
fi
exec "$dest"
