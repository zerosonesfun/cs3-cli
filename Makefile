# cs3 CLI

APP=cs3
PKG=./cmd/cs3
DIST=dist
VERSION?=1.1.1

.PHONY: build release clean tidy

build:
	go build -trimpath -ldflags "-s -w" -o $(APP) $(PKG)

tidy:
	go mod tidy

clean:
	rm -rf $(DIST) $(APP) $(APP).exe

release: tidy
	@mkdir -p $(DIST)
	$(MAKE) _zip GOOS=darwin GOARCH=arm64 EXT= SUFFIX=darwin-arm64 LAUNCHER=darwin
	$(MAKE) _zip GOOS=darwin GOARCH=amd64 EXT= SUFFIX=darwin-amd64 LAUNCHER=darwin
	$(MAKE) _zip GOOS=linux GOARCH=amd64 EXT= SUFFIX=linux-amd64 LAUNCHER=linux
	$(MAKE) _zip GOOS=windows GOARCH=amd64 EXT=.exe SUFFIX=windows-amd64 LAUNCHER=windows
	cd $(DIST) && shasum -a 256 *.zip > SHA256SUMS
	@echo "Release artifacts in $(DIST)/"

_zip:
	@mkdir -p $(DIST)/staging-$(SUFFIX)
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -trimpath -ldflags "-s -w" -o $(DIST)/staging-$(SUFFIX)/$(APP)$(EXT) $(PKG)
	cp packaging/README.txt $(DIST)/staging-$(SUFFIX)/
	@if [ "$(LAUNCHER)" = "darwin" ]; then \
		cp "packaging/darwin/Open CtrlShift3 CLI.command" $(DIST)/staging-$(SUFFIX)/; \
		chmod +x "$(DIST)/staging-$(SUFFIX)/Open CtrlShift3 CLI.command"; \
		chmod +x $(DIST)/staging-$(SUFFIX)/$(APP); \
	elif [ "$(LAUNCHER)" = "linux" ]; then \
		cp packaging/linux/open-cs3.sh $(DIST)/staging-$(SUFFIX)/; \
		chmod +x $(DIST)/staging-$(SUFFIX)/open-cs3.sh $(DIST)/staging-$(SUFFIX)/$(APP); \
	else \
		cp "packaging/windows/Open CtrlShift3 CLI.cmd" $(DIST)/staging-$(SUFFIX)/; \
	fi
	cd $(DIST)/staging-$(SUFFIX) && zip -qr ../cs3-$(SUFFIX).zip .
	rm -rf $(DIST)/staging-$(SUFFIX)
	@echo "built $(DIST)/cs3-$(SUFFIX).zip"
