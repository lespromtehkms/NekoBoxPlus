#!/bin/bash

[ -f ./env_java.sh ] && source ./env_java.sh
source ../buildScript/init/env_ndk.sh

BUILD=".build"

rm -rf $BUILD/android \
	$BUILD/java \
	$BUILD/javac-output \
	$BUILD/src

if [ -z "$GOPATH" ]; then
	GOPATH=$(go env GOPATH)
fi

# --- Wire up custom yggdrasil-go with Android VPN socket protection ---
YGG_DIR="../../yggdrasil-go"
if [ -d "$YGG_DIR/src/core" ]; then
	if grep -q "func (l \*linkTCP) dialerFor(" "$YGG_DIR/src/core/link_tcp.go"; then
		sed -i 's/func (l \*linkTCP) dialerFor(/func (l *linkTCP) origDialerFor(/' "$YGG_DIR/src/core/link_tcp.go"
	fi

	cat > "$YGG_DIR/src/core/singbox_hook.go" << 'EOF'
package core

import (
	"context"
	"net"
	"syscall"
)

func (c *Core) SetSocketControl(fn func(network, address string, rawConn syscall.RawConn) error) {
	c.socketControl = fn
}

func (c *Core) SetCustomDialContext(fn func(ctx context.Context, network, address string) (net.Conn, error)) {
	c.customDialContext = fn
}

func (l *linkTCP) dialerFor(dst *net.TCPAddr, sintf string) (*net.Dialer, error) {
	d, err := l.origDialerFor(dst, sintf)
	if err != nil {
		return nil, err
	}
	if l.links != nil && l.links.core != nil && l.links.core.socketControl != nil {
		orig := d.Control
		ctrl := l.links.core.socketControl
		d.Control = func(network, address string, c syscall.RawConn) error {
			if orig != nil {
				if err := orig(network, address, c); err != nil {
					return err
				}
			}
			return ctrl(network, address, c)
		}
	}
	return d, nil
}
EOF

	if ! grep -q "socketControl" "$YGG_DIR/src/core/core.go"; then
		sed -i 's/outboundAllowed sync.Map/outboundAllowed sync.Map\n\tsocketControl func(network, address string, c syscall.RawConn) error\n\tcustomDialContext func(ctx context.Context, network, address string) (net.Conn, error)/' "$YGG_DIR/src/core/core.go"
		sed -i 's/"sync"/"sync"\n\t"syscall"/' "$YGG_DIR/src/core/core.go"
	fi
fi

if ! grep -q "github.com/yggdrasil-network/yggdrasil-go" ../../sing-box/go.mod; then
	echo "" >> ../../sing-box/go.mod
	echo "require github.com/yggdrasil-network/yggdrasil-go v0.5.15" >> ../../sing-box/go.mod
	echo "replace github.com/yggdrasil-network/yggdrasil-go => ../yggdrasil-go" >> ../../sing-box/go.mod
fi

if ! grep -q "github.com/yggdrasil-network/yggdrasil-go" ./go.mod; then
	echo "" >> ./go.mod
	echo "require github.com/yggdrasil-network/yggdrasil-go v0.5.15" >> ./go.mod
	echo "replace github.com/yggdrasil-network/yggdrasil-go => ../../yggdrasil-go" >> ./go.mod
fi

pushd ../../sing-box >/dev/null
go mod tidy
popd >/dev/null

go mod tidy
# ----------------------------------------------------------------------

# Verify gVisor workaround.
if ! go run -tags=with_gvisor ../../sing-box/cmd/internal/gvisor_workaround_verify/main.go; then
	echo "ERROR: gVisor workaround verification failed"
	exit 1
fi

export GOBIND=gobind

build_adblock_assets() {
	local sing_box_dir="../../sing-box"
	local bridge_dir="$sing_box_dir/common/adblock/bridge"
	local target_dir="$bridge_dir/target"
	local llvm_bin="$ANDROID_NDK_HOME/toolchains/llvm/prebuilt/linux-x86_64/bin"

	if [ ! -f "$sing_box_dir/Makefile.plus" ]; then
		echo "ERROR: sing-box Makefile.plus not found"
		exit 1
	fi

	make -C "$sing_box_dir" -f Makefile.plus adblock-rust-sync adblock-resources-generate || exit 1

	export CARGO_TARGET_DIR="$target_dir"
	export CARGO_TARGET_ARMV7_LINUX_ANDROIDEABI_LINKER="$llvm_bin/armv7a-linux-androideabi21-clang"
	export CARGO_TARGET_AARCH64_LINUX_ANDROID_LINKER="$llvm_bin/aarch64-linux-android21-clang"
	export CARGO_TARGET_I686_LINUX_ANDROID_LINKER="$llvm_bin/i686-linux-android21-clang"
	export CARGO_TARGET_X86_64_LINUX_ANDROID_LINKER="$llvm_bin/x86_64-linux-android21-clang"
	export AR_armv7_linux_androideabi="$llvm_bin/llvm-ar"
	export AR_aarch64_linux_android="$llvm_bin/llvm-ar"
	export AR_i686_linux_android="$llvm_bin/llvm-ar"
	export AR_x86_64_linux_android="$llvm_bin/llvm-ar"

	cargo build --manifest-path "$bridge_dir/Cargo.toml" --release --target armv7-linux-androideabi || exit 1
	cargo build --manifest-path "$bridge_dir/Cargo.toml" --release --target aarch64-linux-android || exit 1
	cargo build --manifest-path "$bridge_dir/Cargo.toml" --release --target i686-linux-android || exit 1
	cargo build --manifest-path "$bridge_dir/Cargo.toml" --release --target x86_64-linux-android || exit 1
}

build_adblock_assets

bash ../buildScript/lib/tproxy.sh || exit 1

resolve_version() {
	local dir="$1" mode="${2:-}" version=""
	[ -d "$dir" ] || return 0
	if [ "$mode" = "tag" ]; then
		version="$(git -C "$dir" describe --tags --abbrev=0 2>/dev/null || true)"
	else
		version="$(git -C "$dir" describe --tags --exact-match 2>/dev/null || true)"
		if [ -z "$version" ]; then
			version="$(git -C "$dir" rev-parse --short HEAD 2>/dev/null || true)"
		fi
	fi
	printf '%s' "$version"
}

VERSION_AMNEZIA="v3.1.20260814"
VERSION_BYEDPI="ba53229"
VERSION_MASTERDNSVPN="v2026.06.13.234407-7de2476"
VERSION_ADBLOCK_RUST="$(resolve_version ../../adblock-rust)"
VERSION_ADBLOCK_RESOURCES="$(resolve_version ../../adblock-resources)"
VERSION_UBLOCK="$(resolve_version ../../uBlock)"
VERSION_SING_BOX="1.14.3-nekoygg"

VERSION_LDFLAGS=""
append_version_ldflag() {
	local symbol="$1" value="$2"
	[ -n "$value" ] && VERSION_LDFLAGS="${VERSION_LDFLAGS} -X libcore.${symbol}=${value}"
}
append_version_ldflag VersionAmnezia "$VERSION_AMNEZIA"
append_version_ldflag VersionByeDPI "$VERSION_BYEDPI"
append_version_ldflag VersionMasterDnsVPN "$VERSION_MASTERDNSVPN"
append_version_ldflag VersionAdblockRust "$VERSION_ADBLOCK_RUST"
append_version_ldflag VersionAdblockResources "$VERSION_ADBLOCK_RESOURCES"
append_version_ldflag VersionUBlock "$VERSION_UBLOCK"
[ -n "$VERSION_SING_BOX" ] && VERSION_LDFLAGS="${VERSION_LDFLAGS} -X github.com/sagernet/sing-box/constant.Version=${VERSION_SING_BOX}"

echo ">> core version: sing-box-plus=${VERSION_SING_BOX:-?}"

gomobile bind -v -androidapi 23 -trimpath -ldflags="-s -w -checklinkname=0${VERSION_LDFLAGS}" -tags='with_conntrack,with_gvisor,with_quic,with_wireguard,with_awg,with_tailscale,with_openvpn,with_openconnect,with_utls,with_clash_api,with_naive_outbound,with_trusttunnel_cronet,with_adblock,with_adblock_cronet,with_grpc,http2legacy,badlinkname,tfogo_checklinkname0' . || exit 1
rm -r libcore-sources.jar

proj=../app/libs
mkdir -p $proj
cp -f libcore.aar $proj
echo ">> install $(realpath $proj)/libcore.aar"
