VERSION ?= dev
LDFLAGS = -ldflags "-X main.version=$(VERSION)"

DIST_DIR = dist
BINARY = agentbox
BIN_PATH = $(DIST_DIR)/$(BINARY)

.PHONY: build snapshot install test race cover lint clean

build:
	@go build $(LDFLAGS) -o $(BIN_PATH) ./cmd/agentbox

snapshot:
	@goreleaser release --snapshot --skip=publish --clean

install: build
	@mkdir -p ~/.local/bin
	@rm -f ~/.local/bin/$(BINARY)
	@cp $(BIN_PATH) ~/.local/bin/

test:
	@go test -timeout 3m -v ./...

race:
	@go test -race -timeout 3m ./...

cover:
	go test -race -coverprofile=coverage.out -timeout 3m ./...
	go tool cover -func=coverage.out
	@echo "---"
	@echo "HTML report: go tool cover -html=coverage.out"

lint:
	@golangci-lint run

clean:
	@rm -rf $(DIST_DIR) coverage.out
