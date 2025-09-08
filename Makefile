APP := minimark

.PHONY: all build install run clean test cover

all: build

build:
	go build -o bin/$(APP) .

install:
	go install .

run:
	go run .

clean:
	rm -rf bin

test:
	go test ./...

cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out
