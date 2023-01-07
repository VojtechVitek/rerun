.PHONY: help
help:
	@cat Makefile | grep '^[a-z]*[:]$$'

.PHONY: install
install:
	go install ./cmd/rerun

.PHONY: build
build:
	@rm -rf ./bin/*
	@mkdir -p ./bin
	go build -o ./bin/rerun ./cmd/rerun

.PHONY: test
test:
	go test ./...

.PHONY: test
run:
	go run github.com/goware/rerun/cmd/rerun -watch ./ -ignore bin -run 'echo hi && sleep 100000000'
