#!/usr/bin/env bash
set -euo pipefail

ROOT="$(pwd)"
NBA="$ROOT/workspace/NekoBoxForAndroid"

# --- clone custom yggdrasil-go into workspace/yggdrasil-go -----------
if [ ! -d "$ROOT/workspace/yggdrasil-go" ]; then
  echo "Cloning lespromtehkms/yggdrasil-go..."
  git clone --depth 1 -b develop https://github.com/lespromtehkms/yggdrasil-go.git "$ROOT/workspace/yggdrasil-go" || \
  git clone --depth 1 https://github.com/lespromtehkms/yggdrasil-go.git "$ROOT/workspace/yggdrasil-go"
fi

cd "$NBA"

# --- signing ---------------------------------------------------------
if [ -n "${KEYSTORE_BASE64:-}" ]; then
  echo "$KEYSTORE_BASE64" | base64 -d > release.keystore
  echo "using the provided KEYSTORE_BASE64 keystore"
else
  echo "Generating self-signed release.keystore for NekoYgg..."
  rm -f release.keystore
  keytool -genkeypair -v \
    -keystore release.keystore \
    -alias nekoygg \
    -keyalg RSA \
    -keysize 2048 \
    -validity 10000 \
    -storepass nekoygg123 \
    -keypass nekoygg123 \
    -dname "CN=NekoYgg, OU=Dev, O=NekoYgg, L=Unknown, ST=Unknown, C=US"
  export KEYSTORE_PASS="nekoygg123"
  export ALIAS_NAME="nekoygg"
  export ALIAS_PASS="nekoygg123"
fi

# --- native core (Docker) --------------------------------------------
bash buildScript/lib/assets.sh

mkdir -p "$ROOT/workspace/sing-box/common/adblock/adblockrust/resources/files"
touch "$ROOT/workspace/sing-box/common/adblock/adblockrust/resources/files/placeholder.txt"

bash buildScript/lib/core.docker.sh

[ -f app/libs/libcore.aar ] || { echo "app/libs/libcore.aar missing after core.docker.sh" >&2; exit 1; }

# --- gradle -------------------------------------------------------------
NDK_DIR="${ANDROID_NDK_HOME:-$(find "$ANDROID_HOME/ndk" -mindepth 1 -maxdepth 1 -type d | sort -V | tail -n1)}"
{
  echo "sdk.dir=${ANDROID_HOME}"
  echo "ndk.dir=${NDK_DIR}"
  echo "KEYSTORE_PASS=${KEYSTORE_PASS:-nekoygg123}"
  echo "ALIAS_NAME=${ALIAS_NAME:-nekoygg}"
  echo "ALIAS_PASS=${ALIAS_PASS:-nekoygg123}"
} > local.properties

./gradlew app:assemblePlusRelease --stacktrace

# --- collect + report version ------------------------------------------
OUT_DIR="$ROOT/dist"
mkdir -p "$OUT_DIR"
find app/build/outputs/apk/plus/release -name '*.apk' -exec cp -v {} "$OUT_DIR/" \;

VERSION_NAME="$(grep -oP '^VERSION_NAME=\K.*' nb4a.properties)"
echo "Version: $VERSION_NAME"
if [ -n "${GITHUB_OUTPUT:-}" ]; then
  echo "version_name=$VERSION_NAME" >> "$GITHUB_OUTPUT"
fi

echo
echo "Built:"
ls -la "$OUT_DIR"
