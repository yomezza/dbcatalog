#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
if [[ $(uname -s) != Linux || $(uname -m) != x86_64 ]]; then
  echo 'Этот сценарий выпуска предназначен для Linux x86_64.' >&2
  exit 1
fi
pkg-config --exists gtk+-3.0 webkit2gtk-4.1
npm --prefix frontend ci
npm --prefix frontend test
npm --prefix frontend run build
go test ./...
go vet ./...
# Both OS builds use the same UI and App bindings. No generated bridge is required.
wails build -platform linux/amd64 -tags webkit2_41 -s -skipbindings
version=$(sed -n 's/.*"productVersion": "\([^"]*\)".*/\1/p' wails.json)
stage=$(mktemp -d)
trap 'rm -rf "$stage"' EXIT
mkdir -p "$stage/CatalogDB"
install -m 755 build/bin/CatalogDB "$stage/CatalogDB/CatalogDB"
install -m 644 packaging/linux/catalogdb.desktop "$stage/CatalogDB/"
install -m 644 build/appicon.png "$stage/CatalogDB/catalogdb.png"
install -m 644 docs/linux.md "$stage/CatalogDB/README.md"
install -m 644 docs/pdf-notices.txt "$stage/CatalogDB/PDF-NOTICES.txt"
# No credentials or local connection settings are included.
tar --owner=0 --group=0 --numeric-owner -czf "build/bin/CatalogDB-linux-x64-$version.tar.gz" -C "$stage" CatalogDB
(
 cd build/bin
 sha256sum "CatalogDB-linux-x64-$version.tar.gz" > "CatalogDB-linux-x64-$version.tar.gz.sha256"
)
