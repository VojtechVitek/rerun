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

.PHONY: dist
dist:
	@rm -rf ./dist/*
	@mkdir -p ./dist
	GOOS=darwin GOARCH=amd64 go build -o ./bin/rerun-darwin-amd64 ./cmd/rerun
	GOOS=darwin GOARCH=arm64 go build -o ./bin/rerun-darwin-arm64 ./cmd/rerun
	GOOS=linux GOARCH=amd64 go build -o ./bin/rerun-linux-amd64 ./cmd/rerun
	GOOS=linux GOARCH=386 go build -o ./bin/rerun-linux-386 ./cmd/rerun
	#GOOS=windows GOARCH=amd64 go build -o ./bin/rerun-windows-amd64.exe ./cmd/rerun
	#GOOS=windows GOARCH=386 go build -o ./bin/rerun-windows-386.exe ./cmd/rerun

.PHONY: test
test:
	go test ./...
