# LokroNet – Build & Release
# Linux-Server baut damit, Windows nutzt ersatzweise die Powershell-Zeilen unten.
VERSION := $(shell cat VERSION 2>/dev/null || echo 0.1.0-dev)
MODULE  := github.com/lokro/lokronet
LDFLAGS := -s -w -X main.Version=$(VERSION)
DIST    := dist

.PHONY: all build-linux build-all checksums clean version

version:
	@echo $(VERSION)

# Linux-Binaries (für net.lokro.dev/dl). Läuft auf jedem OS mit Go-Toolchain.
build-linux:
	mkdir -p $(DIST)
	GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST)/lokronet_linux_amd64 ./cmd/lokronet
	GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST)/lokronet_linux_arm64 ./cmd/lokronet
	cd $(DIST) && tar -czf lokronet_$(VERSION)_linux_amd64.tar.gz lokronet_linux_amd64
	cd $(DIST) && tar -czf lokronet_$(VERSION)_linux_arm64.tar.gz lokronet_linux_arm64

# Alles (Linux + Windows), für Releases.
build-all: build-linux
	mkdir -p $(DIST)
	GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST)/lokronet_windows_amd64.exe ./cmd/lokronet

# Windows-Zip fürs Release (auf Linux: zip, auf Windows-Powershell: Compress-Archive).
build-windows:
	mkdir -p $(DIST)
	GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST)/lokronet_windows_amd64.exe ./cmd/lokronet
	cd $(DIST) && zip -j lokronet_$(VERSION)_windows_amd64.zip lokronet_windows_amd64.exe

# checksums.txt für die Installer (Verifikation per sha256).
checksums:
	cd $(DIST) && sha256sum lokronet_$(VERSION)_linux_*.tar.gz lokronet_$(VERSION)_windows_*.zip > checksums.txt
	cat $(DIST)/checksums.txt

clean:
	rm -rf $(DIST)

# GitHub-Pages-Sync: published /install + /install.ps1 unter net.lokro.dev.
# Quelle der Wahrheit bleibt deploy/ (hier nur kopieren, nie editieren).
pages:
	mkdir -p docs
	cp deploy/install.sh docs/install
	cp deploy/install.ps1 docs/install.ps1
	touch docs/.nojekyll
	@echo "docs/install + docs/install.ps1 synchronisiert (CNAME + index.html sind eingecheckt)"

# Windows ohne make (Powershell):
#   $v = (Get-Content VERSION).Trim()
#   $env:GOOS="linux"; $env:GOARCH="amd64"; go build -trimpath -ldflags "-s -w -X main.Version=$v" -o dist/lokronet_linux_amd64 ./cmd/lokronet
