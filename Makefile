APP := bmcptools
VERSION ?= dev

# Build flags: inject version when VERSION is set; always strip debug info.
ifneq ($(VERSION),)
    LDFLAGS := -s -w -X main.serverVersion=$(VERSION)
else
    LDFLAGS := -s -w
endif

# ── OS-specific settings ─────────────────────────────────────────────────────
ifeq ($(OS),Windows_NT)
    BINARY := bin/$(APP).exe
    RM     := if exist "$(subst /,\,$(BINARY))" del /f /q "$(subst /,\,$(BINARY))"
else
    BINARY := bin/$(APP)
    RM     := rm -f $(BINARY)
endif

# ── Targets ──────────────────────────────────────────────────────────────────

.PHONY: build install vet lint test clean

## build: compile the binary into ./bin/ (pass VERSION=<tag> to stamp the version)
build: vet
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) .
	@echo "Built $(BINARY)$(if $(VERSION), ($(VERSION)),)"

## install: install into GOBIN (pass VERSION=<tag> to stamp the version)
install: vet
	go install -ldflags "$(LDFLAGS)" .
	@echo "Installed $(APP)$(if $(VERSION), ($(VERSION)),)"

## vet: run go vet
vet:
	go vet ./...

## lint: run golangci-lint (requires golangci-lint to be installed)
lint:
	golangci-lint run ./...

## test: run all tests
test:
	go test -count=1 ./...

## clean: remove the compiled binary from ./bin/
clean:
	$(RM)
