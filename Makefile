VERSION ?= dev
LDFLAGS = -ldflags "-X main.version=$(VERSION)"

.PHONY: build install test race cover lint clean docker

build:
	go build $(LDFLAGS) -o agentbox ./cmd/agentbox

install: build
	mkdir -p ~/.local/bin
	rm ~/.local/bin/agentbox
	cp agentbox ~/.local/bin/

test:
	go test -timeout 3m -v ./...

race:
	go test -race -timeout 3m ./...

cover:
	go test -race -coverprofile=coverage.out -timeout 3m ./...
	go tool cover -func=coverage.out
	@echo "---"
	@echo "HTML report: go tool cover -html=coverage.out"

lint:
	golangci-lint run

clean:
	rm -f agentbox coverage.out
