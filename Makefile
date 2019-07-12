all: test-ci

setup:
	go get golang.org/x/lint/golint
	go get github.com/mattn/goveralls

compile:
	go build ./...

lint:
	golint ./... | { grep -vwE "exported (var|function|method|type|const) \S+ should have comment" || true; }

format:
	go fmt ./...

run:
	./compressiontest

test-ci: compile lint format run
