.PHONY: test
test:
	@go test -race -covermode=atomic -coverprofile=coverage.txt $(extend) ./...

.PHONY: benchmark
benchmark:
	@go test -run='^$$' -bench=. $(extend) ./...

.PHONY: lint
lint:
	@golangci-lint run ./...

.PHONY: tools
tools:
	go install github.com/cweill/gotests/gotests@latest
	go install github.com/fatih/gomodifytags@latest
	go install github.com/josharian/impl@latest
	go install github.com/haya14busa/goplay/cmd/goplay@latest
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0
	go install golang.org/x/tools/gopls@latest
