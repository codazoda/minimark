APP := minimark
DIST := dist
PLATFORMS := darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64 windows/arm64

.PHONY: all build build-local install run clean test cover build-all dist-clean

all: build install

install:
	go install .

run:
	go run .

clean:
	rm -rf $(DIST)

build:
	@for platform in $(PLATFORMS); do \
		os=$${platform%/*}; arch=$${platform##*/}; \
		outdir=$(DIST)/$$os-$$arch; \
		outfile=$(APP); \
		if [ "$$os" = "windows" ]; then outfile=$(APP).exe; fi; \
		echo "Building $$os/$$arch -> $$outdir/$$outfile"; \
		mkdir -p $$outdir; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -o $$outdir/$$outfile . || exit 1; \
	done

test:
	go test ./...

cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out
