# Minimark

Minimark is a Minimal Markdown editor for web publishing.


## Usage

From any directory with Markdown files, run:

```sh
minimark
```

Then open `http://localhost:8080/`.

- Loads the most recently modified `.md` file in the current directory (creates `untitled.md` if none exist).
- Autosaves the file after 500ms of inactivity while typing.
- Serves a minimal UI (HTML/CSS/JS) embedded in the binary—no extra files are written in your working directory.

Optional flag:

```sh
minimark -addr localhost:8080
```


## Build and Install

Requirements: Go 1.21+

Using Makefile targets:

```sh
# Build locally (outputs bin/minimark)
make build

# Install globally (uses go install to GOBIN or GOPATH/bin)
make install

# Run in place for development
make run
```

Ensure `GOBIN` (or `GOPATH/bin`) is on your `PATH` to run the installed binary from anywhere.


## Notes

- All UI assets in `_includes/` are embedded into the binary via `go:embed`.
- Only Markdown files live in your working directory for a clean setup.
