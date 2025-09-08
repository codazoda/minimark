# Minimark

A minimal Markdown editor for web publishing.

It saves edits automatically and generates html automatically if `cmark-gfm` is found.

**It's currently in a pre-released (alpha) stage and it could be dangerous to use. Use it only on directories that are committed to source control and backup often. It's advanced enough to edit it's own web page, however.**


## Usage

From any directory with Markdown files, run:

```sh
minimark
```

Then open `http://localhost:8080/`.

- Loads the most recently modified `.md` file in the current directory (creates `untitled.md` if none exist).
- Autosaves the file after 500ms of inactivity while typing.
- Serves a minimal UI (HTML/CSS/JS) embedded in the binary—no extra files are written in your working directory.

### File Naming and Renaming

Minimark tries to keep filenames readable and in sync with your document title:

- New files start as `untitled.md` (or `untitled-1.md`, `untitled-2.md`, … if a file already exists).
- On save, if the file is not `index.md` or `readme.md`, the first level‑1 heading (H1) determines the filename.
  - Supported H1 formats: `# Title` (ATX) and the Setext style:
    ```
    Title
    =====
    ```
  - The title is slugified (lowercase, spaces/punctuation → dashes) to become the filename, e.g. “My Note” → `my-note.md`.
- If the slugged name differs from the current file, Minimark renames the file on save.
  - Existing files are never overwritten; a unique suffix is added (`-1`, `-2`, …) if needed.
  - The old Markdown file is deleted after a successful rename (and its previously exported HTML is also removed).
- Special cases that never auto‑rename: `index.md` and `readme.md`.

HTML export filenames under `docs/` follow these rules:

- For most files, `name.md` → `docs/name.html`.
- Special case: `readme.md` exports to `docs/index.html` if there is no `index.md` in the directory.
- Optional wrapping with `_includes/header.html` and `_includes/footer.html` if present.


### HTML Export (cmark-gfm)

If `cmark-gfm` is installed and in your `PATH`, Minimark will automatically export the current file as an HTML file under `./docs` after each save.

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


## Build and Install for Development

Requirements: Go 1.21+

Using Makefile targets:

```sh
# Build for all platforms and install locally
make

# Run in place for development
make run

# Build for all platforms
make build

# Run all tests
make test

# Install globally (uses go install to GOBIN or GOPATH/bin)
make install

# See code coverage
make cover

# Delete all build files
make clean
```

Ensure `GOBIN` (or `GOPATH/bin`) is on your `PATH` to run the installed binary from anywhere.


## Notes

- All UI assets in `static/` are embedded into the binary via `go:embed`.
- Only Markdown files live in your working directory for a clean setup.
