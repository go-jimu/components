.PHONY: test
test: 
	@go test -race -covermode=atomic -v -coverprofile=coverage.txt ./... ;

.PHONY: benchmark
benchmark: 
	@go test -bench=. -run=^Benchmark ./...;
