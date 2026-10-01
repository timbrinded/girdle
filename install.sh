#!/bin/sh
# Installs Girdle from its GitHub releases:
#
#   curl -fsSL https://raw.githubusercontent.com/timbrinded/girdle/master/install.sh | sh
#
# GIRDLE_VERSION picks a release, such as v0.1.0 (default: the latest).
# GIRDLE_INSTALL_DIR picks where the binary goes (default: ~/.local/bin).
set -eu

repo=timbrinded/girdle

fail() {
	echo "girdle install: $*" >&2
	exit 1
}

fetch() {
	if command -v curl >/dev/null 2>&1; then
		curl -fsSL "$1" -o "$2"
	elif command -v wget >/dev/null 2>&1; then
		wget -qO "$2" "$1"
	else
		fail "needs curl or wget"
	fi
}

sha256() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | cut -d' ' -f1
	else
		shasum -a 256 "$1" | cut -d' ' -f1
	fi
}

# Everything runs from main, so a download cut short runs nothing.
main() {
	case $(uname -s) in
	Linux) os=linux ;;
	Darwin) os=darwin ;;
	*) fail "no release for $(uname -s); build from source with: go install github.com/$repo/cmd/girdle@latest" ;;
	esac
	case $(uname -m) in
	x86_64 | amd64) arch=amd64 ;;
	arm64 | aarch64) arch=arm64 ;;
	*) fail "no release for $(uname -m); build from source with: go install github.com/$repo/cmd/girdle@latest" ;;
	esac

	version=${GIRDLE_VERSION:-latest}
	if [ "$version" = latest ]; then
		base=https://github.com/$repo/releases/latest/download
	else
		base=https://github.com/$repo/releases/download/$version
	fi
	dir=${GIRDLE_INSTALL_DIR:-$HOME/.local/bin}
	archive=girdle_${os}_${arch}.tar.gz

	tmp=$(mktemp -d)
	trap 'rm -rf "$tmp"' EXIT
	echo "Downloading girdle ($version) for $os/$arch"
	fetch "$base/$archive" "$tmp/$archive" || fail "couldn't download $base/$archive"
	fetch "$base/checksums.txt" "$tmp/checksums.txt" || fail "couldn't download $base/checksums.txt"
	want=$(grep " $archive\$" "$tmp/checksums.txt" | cut -d' ' -f1)
	[ -n "$want" ] && [ "$want" = "$(sha256 "$tmp/$archive")" ] || fail "checksum mismatch for $archive"

	tar -xzf "$tmp/$archive" -C "$tmp" girdle
	mkdir -p "$dir"
	install -m 755 "$tmp/girdle" "$dir/girdle"
	echo "Installed $("$dir/girdle" -version) to $dir/girdle"
	case ":$PATH:" in
	*":$dir:"*) ;;
	*) echo "Add $dir to your PATH, for example: export PATH=\"$dir:\$PATH\"" ;;
	esac
}

main
