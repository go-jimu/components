.PHONY: test
test:
	@go test -race -covermode=atomic -coverprofile=coverage.txt $(extend) ./...

.PHONY: benchmark
benchmark:
	@go test -run='^$$' -bench=. $(extend) ./...

.PHONY: tools
tools:
	go install github.com/cweill/gotests/gotests@latest
	go install github.com/fatih/gomodifytags@latest
	go install github.com/josharian/impl@latest
	go install github.com/haya14busa/goplay/cmd/goplay@latest
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install golang.org/x/tools/gopls@latest
