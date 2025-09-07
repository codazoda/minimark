APP := minimark

.PHONY: all build install run clean

all: build

build:
	go build -o bin/$(APP) .

install:
	go install .

run:
	go run .

clean:
	rm -rf bin

