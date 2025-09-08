# Minimark

Minimark is a Minimal Markdown editor for web publishing.

It's currently in a pre-released (alpha) stage and it could be dangerous to use. Use it only on directories that are committed to source control and backup often. It's advanced enough to edit it's own web page, however.


## Usage

From any directory with Markdown files, run:

```sh
minimark
```

Then open `http://localhost:8080/`.

- Loads the most recently modified `.md` file in the current directory (creates `untitled.md` if none exist).
- Autosaves the file after 500ms of inactivity while typing.
- Serves a minimal UI (HTML/CSS/JS) embedded in the binary—no extra files are written in your working directory.

### HTML Export (cmark-gfm)

If `cmark-gfm` is installed and on your `PATH`, Minimark will automatically export the current file you are editing to an HTML file under `./docs` after each save.

You can disable automatic export with the `-export=false` flag:

```sh
minimark -export=false
```

#### Optional header/footer and static includes

- Place `header.html` and/or `footer.html` in a local `_includes/` directory. On export, Minimark wraps the converted HTML as:
  - `header.html` (if present) + converted Markdown + `footer.html` (if present)
- On startup, all files inside your local `_includes/` are copied into `./docs` (recursively). Use this to ship CSS/JS/images referenced by your header/footer.
- If `_includes/` is missing, wrapping is skipped and no files are copied.
 - Special case: exporting `readme.md` writes `docs/index.html` if there is no `index.md` in the directory.

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

- All UI assets in `static/` are embedded into the binary via `go:embed`.
- Only Markdown files live in your working directory for a clean setup.
