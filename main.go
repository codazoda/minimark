package main

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

//go:embed static/*
var embeddedIncludes embed.FS

func main() {
	addr := flag.String("addr", "localhost:8080", "address to listen on, e.g. localhost:8080 or 127.0.0.1:8080")
	exportHTML := flag.Bool("export", true, "export HTML to ./docs using cmark-gfm on save")
	flag.Parse()

	http.Handle("/", rootHandler())
	http.HandleFunc("/new", handleNew)
	http.HandleFunc("/open", openLastMarkdown)
	http.HandleFunc("/index", handleLoadIndex)
	http.HandleFunc("/save", handleSave)
	http.HandleFunc("/lock", handleLock)
	http.HandleFunc("/unlock", handleUnlock)

	// Discover cmark-gfm availability
	if *exportHTML {
		if path, err := exec.LookPath("cmark-gfm"); err == nil {
			cmarkPath = path
			log.Printf("cmark-gfm found at %s; will export HTML on save.", path)
		} else {
			log.Printf("cmark-gfm not found; docs will not be exported. Install cmark-gfm to enable exports.")
		}
	} else {
		log.Printf("HTML export disabled by flag.")
	}

	// Copy any local includes to docs on startup (best-effort)
	if err := copyIncludesToDocs("_includes", "docs"); err != nil {
		log.Printf("copy includes failed: %v", err)
	}

	log.Printf("Serving embedded UI on http://%s\n", *addr)
	if err := http.ListenAndServe(*addr, nil); err != nil {
		log.Fatal(err)
	}
}

func rootHandler() http.Handler {
	sub, err := fs.Sub(embeddedIncludes, "static")
	if err != nil {
		// If embedding misconfigured, fail loudly at runtime
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "embedded assets not found", http.StatusInternalServerError)
		})
	}
	return http.FileServer(http.FS(sub))
}

// handleLoadIndex streams the contents of ./index.md as text/plain.
func handleLoadIndex(w http.ResponseWriter, r *http.Request) {
	const indexPath = "index.md"
	b, err := os.ReadFile(indexPath)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "index.md not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Filename", filepath.Base(indexPath))
	if _, err := w.Write(b); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// handleSave writes the request body to the given file in the current
// directory. The target filename is resolved from the `file` query param,
// then `X-Filename` header, and defaults to "index.md". Only basenames are
// allowed to avoid path traversal.
func handleSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name := r.URL.Query().Get("file")
	if name == "" {
		name = r.Header.Get("X-Filename")
	}
	if name == "" {
		name = "index.md"
	}
	// Prevent path traversal by ensuring basename only.
	if filepath.Base(name) != name {
		http.Error(w, "invalid filename", http.StatusBadRequest)
		return
	}
	// Read full body.
	data, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// Require a valid lock token
	token := r.Header.Get("X-Lock")
	if !hasValidLock(name, token) {
		http.Error(w, "file is locked by another editor", http.StatusLocked)
		return
	}
	// Decide final target filename based on first H1, unless reserved
	targetName := decideFilenameFromContent(name, data)
	// If renaming, avoid overwriting any existing file by picking a unique name
	if targetName != name {
		targetName = uniqueAvailableName(targetName)
	}
	if err := os.WriteFile(targetName, data, 0644); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// If we renamed, remove the previous file and its exported HTML (best-effort).
	if targetName != name {
		_ = os.Remove(name)
		// Compute old HTML out name using current mapping rules
		oldOutName := htmlOutNameFor(filepath.Base(name))
		_ = os.Remove(filepath.Join("docs", oldOutName))
	}
	// Trigger export after save if available/enabled for this file only
	if cmarkPath != "" {
		outName := htmlOutNameFor(filepath.Base(targetName))
		outPath := filepath.Join("docs", outName)
		if err := exportMarkdownTo(cmarkPath, targetName, outPath); err != nil {
			log.Printf("export error for %s: %v", targetName, err)
		}
	}
	// Return the filename so the client can update state
	w.Header().Set("X-Filename", filepath.Base(targetName))
	w.WriteHeader(http.StatusNoContent)
}

var cmarkPath string // discovered at startup if available

// htmlOutNameFor computes the output HTML filename for a given markdown basename.
// Special-case: readme.md -> index.html if no index.md exists.
func htmlOutNameFor(mdBase string) string {
	if strings.EqualFold(mdBase, "readme.md") && !fileExistsLower("index.md") {
		return "index.html"
	}
	return strings.TrimSuffix(mdBase, filepath.Ext(mdBase)) + ".html"
}

// exportMarkdownTo converts a single Markdown file to HTML using cmark-gfm and
// writes it to outPath, wrapping with optional _includes/header/footer.
func exportMarkdownTo(cmark, src, outPath string) error {
	if !strings.EqualFold(filepath.Ext(src), ".md") {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		return err
	}
	cmd := exec.Command(cmark, src)
	body, err := cmd.Output()
	if err != nil {
		return err
	}
	var header, footer []byte
	if b, err := os.ReadFile(filepath.Join("_includes", "header.html")); err == nil {
		header = b
	}
	if b, err := os.ReadFile(filepath.Join("_includes", "footer.html")); err == nil {
		footer = b
	}
	composed := make([]byte, 0, len(header)+len(body)+len(footer))
	composed = append(composed, header...)
	composed = append(composed, body...)
	composed = append(composed, footer...)
	return os.WriteFile(outPath, composed, 0644)
}

// fileExistsLower checks for a file in the current directory by lowercased name.
func fileExistsLower(name string) bool {
	want := strings.ToLower(name)
	entries, err := os.ReadDir(".")
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.ToLower(e.Name()) == want {
			return true
		}
	}
	return false
}

var atxH1Re = regexp.MustCompile(`(?m)^\s*#\s+(.+?)\s*$`)
var setextH1Re = regexp.MustCompile(`(?m)^\s*([^\r\n]+?)\s*\r?\n[ \t]*=+[ \t]*$`)

func extractTitle(content []byte) string {
	s := string(content)
	atxIdx := atxH1Re.FindStringSubmatchIndex(s)
	setextIdx := setextH1Re.FindStringSubmatchIndex(s)
	if atxIdx == nil && setextIdx == nil {
		return ""
	}
	if setextIdx == nil || (atxIdx != nil && atxIdx[0] < setextIdx[0]) {
		// Use ATX match
		return strings.TrimSpace(s[atxIdx[2]:atxIdx[3]])
	}
	// Use Setext match
	return strings.TrimSpace(s[setextIdx[2]:setextIdx[3]])
}

// decideFilenameFromContent returns a filename to write to, possibly renamed
// from the first H1 in the content. It never renames index.md or readme.md.
func decideFilenameFromContent(current string, content []byte) string {
	base := filepath.Base(current)
	lower := strings.ToLower(base)
	if lower == "index.md" || lower == "readme.md" {
		return base
	}
	// Look for first ATX H1: lines starting with '# '
	title := extractTitle(content)
	if title == "" {
		return base
	}
	slug := slugify(title)
	if slug == "" {
		return base
	}
	candidate := slug + ".md"
	if candidate == base {
		return base
	}
	return candidate
}

func slugify(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	prevHyphen := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			prevHyphen = false
		} else {
			if !prevHyphen {
				b.WriteRune('-')
				prevHyphen = true
			}
		}
	}
	out := b.String()
	out = strings.Trim(out, "-")
	return out
}

// uniqueAvailableName returns a filename that does not currently exist by
// appending -1, -2, ... to the basename if needed. Only operates on basenames.
func uniqueAvailableName(preferred string) string {
	preferred = filepath.Base(preferred)
	ext := filepath.Ext(preferred)
	base := strings.TrimSuffix(preferred, ext)
	if _, err := os.Stat(preferred); os.IsNotExist(err) {
		return preferred
	}
	for i := 1; ; i++ {
		candidate := fmt.Sprintf("%s-%d%s", base, i, ext)
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
}

// copyIncludesToDocs copies all files and folders from srcDir (e.g. "_includes")
// into dstDir (e.g. "docs"). If srcDir doesn't exist, it does nothing.
func copyIncludesToDocs(srcDir, dstDir string) error {
	info, err := os.Stat(srcDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !info.IsDir() {
		return nil
	}
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return err
	}
	return copyTree(srcDir, dstDir)
}

func copyTree(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		sPath := filepath.Join(src, e.Name())
		dPath := filepath.Join(dst, e.Name())
		if e.IsDir() {
			if err := os.MkdirAll(dPath, 0755); err != nil {
				return err
			}
			if err := copyTree(sPath, dPath); err != nil {
				return err
			}
			continue
		}
		// Copy file
		if err := copyFile(sPath, dPath); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	// Best-effort to copy file mode
	if fi, err := os.Stat(src); err == nil {
		_ = os.Chmod(dst, fi.Mode())
	}
	return nil
}

// --------- Simple per-file locks with 1s TTL ---------

type lockInfo struct {
	token   string
	expires time.Time
}

var (
	locks   = make(map[string]lockInfo)
	locksMu sync.Mutex
)

const lockTTL = time.Second

func handleLock(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name := filepath.Base(r.URL.Query().Get("file"))
	if name == "" {
		http.Error(w, "missing file", http.StatusBadRequest)
		return
	}
	reqToken := r.Header.Get("X-Lock")
	now := time.Now()

	locksMu.Lock()
	defer locksMu.Unlock()

	li, exists := locks[name]
	if exists && now.Before(li.expires) {
		if reqToken != "" && reqToken == li.token {
			// Refresh lock
			li.expires = now.Add(lockTTL)
			locks[name] = li
			w.Header().Set("X-Lock", li.token)
			w.WriteHeader(http.StatusOK)
			return
		}
		// Locked by someone else
		http.Error(w, "locked", http.StatusLocked)
		return
	}
	// Acquire new lock
	tok := reqToken
	if tok == "" {
		tok = newToken()
	}
	locks[name] = lockInfo{token: tok, expires: now.Add(lockTTL)}
	w.Header().Set("X-Lock", tok)
	w.WriteHeader(http.StatusCreated)
}

func handleUnlock(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name := filepath.Base(r.URL.Query().Get("file"))
	tok := r.Header.Get("X-Lock")
	locksMu.Lock()
	defer locksMu.Unlock()
	if li, ok := locks[name]; ok && li.token == tok {
		delete(locks, name)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.Error(w, "not lock owner", http.StatusLocked)
}

func hasValidLock(name, tok string) bool {
	name = filepath.Base(name)
	now := time.Now()
	locksMu.Lock()
	defer locksMu.Unlock()
	li, ok := locks[name]
	if !ok {
		return false
	}
	if now.After(li.expires) {
		delete(locks, name)
		return false
	}
	return li.token == tok
}

func transferLock(oldName, newName, tok string) {
	oldName = filepath.Base(oldName)
	newName = filepath.Base(newName)
	now := time.Now()
	locksMu.Lock()
	defer locksMu.Unlock()
	li, ok := locks[oldName]
	if !ok || li.token != tok || now.After(li.expires) {
		return
	}
	delete(locks, oldName)
	li.expires = now.Add(lockTTL)
	locks[newName] = li
}

func newToken() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// handleNew creates a new file named "untitled.new" in the current working
// directory if it does not already exist. It responds with the file path.
func handleNew(w http.ResponseWriter, r *http.Request) {
	path, created, err := createFileIfNotExists("untitled.new")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(path))
}

// openLastMarkdown locates the most recently modified .md file in the current
// working directory. If none exist, it creates "untitled.new" and opens that.
// It streams the file contents as text/plain.
func openLastMarkdown(w http.ResponseWriter, r *http.Request) {
	file, err := findLastMarkdownFile(".")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if file == "" {
		var created bool
		file, created, err = createFileIfNotExists("untitled.md")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_ = created // not used further; just informational
	}

	b, err := os.ReadFile(file)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Filename", filepath.Base(file))
	if _, err := w.Write(b); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// createFileIfNotExists ensures a file with the given name exists in the
// current working directory. It returns the path, whether it was created, and an error.
func createFileIfNotExists(name string) (string, bool, error) {
	if _, err := os.Stat(name); err == nil {
		return name, false, nil
	} else if !os.IsNotExist(err) {
		return "", false, err
	}
	f, err := os.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return "", false, err
	}
	defer f.Close()
	return name, true, nil
}

// findLastMarkdownFile returns the path to the most recently modified .md file
// in dir. Returns empty string if none found.
func findLastMarkdownFile(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	var latestPath string
	var latestTime time.Time
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.EqualFold(filepath.Ext(name), ".md") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		mt := info.ModTime()
		if latestPath == "" || mt.After(latestTime) {
			latestPath = filepath.Join(dir, name)
			latestTime = mt
		}
	}
	return latestPath, nil
}
