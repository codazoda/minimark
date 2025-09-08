package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// chdirTemp switches to a new temp dir and returns a cleanup that restores cwd.
func chdirTemp(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	return dir
}

func TestRootHandlerServesIndex(t *testing.T) {
	// Uses embedded assets; just ensure it serves index.html
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rootHandler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("unexpected content-type: %q", ct)
	}
	if !strings.Contains(rr.Body.String(), "<html") {
		t.Fatalf("expected html body")
	}
}

func TestHandleLoadIndex_NotFoundAndOK(t *testing.T) {
	chdirTemp(t)
	// Not found
	{
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/index", nil)
		handleLoadIndex(rr, req)
		if rr.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rr.Code)
		}
	}
	// Create index.md and expect 200 + echo content
	const content = "hello"
	if err := os.WriteFile("index.md", []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/index", nil)
	handleLoadIndex(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	if got := rr.Header().Get("X-Filename"); got != "index.md" {
		t.Fatalf("X-Filename = %q", got)
	}
	if rr.Body.String() != content {
		t.Fatalf("body = %q", rr.Body.String())
	}
}

type errWriter struct {
	h    http.Header
	code int
}

func (e *errWriter) Header() http.Header {
	if e.h == nil {
		e.h = make(http.Header)
	}
	return e.h
}
func (e *errWriter) Write([]byte) (int, error)  { return 0, io.ErrClosedPipe }
func (e *errWriter) WriteHeader(statusCode int) { e.code = statusCode }

func TestHandleLoadIndex_WriteError(t *testing.T) {
	chdirTemp(t)
	if err := os.WriteFile("index.md", []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	ew := &errWriter{}
	req := httptest.NewRequest(http.MethodGet, "/index", nil)
	handleLoadIndex(ew, req)
	if ew.code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", ew.code)
	}
}

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"Hello World":         "hello-world",
		"Go 1.22":             "go-1-22",
		" multiple   spaces ": "multiple-spaces",
		"--punctuation!!":     "punctuation",
		"UPPER_lower":         "upper-lower",
	}
	for in, want := range cases {
		if got := slugify(in); got != want {
			t.Errorf("slugify(%q) = %q; want %q", in, got, want)
		}
	}
}

func TestExtractTitle(t *testing.T) {
	atx := []byte("# Title\n\ntext")
	if got := extractTitle(atx); got != "Title" {
		t.Fatalf("got %q", got)
	}
	setext := []byte("Title\n===\n")
	if got := extractTitle(setext); got != "Title" {
		t.Fatalf("got %q", got)
	}
	none := []byte("no title")
	if got := extractTitle(none); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestDecideFilenameFromContent(t *testing.T) {
	tests := []struct {
		name    string
		current string
		content string
		want    string
	}{
		{"reserved-index", "index.md", "# Anything", "index.md"},
		{"reserved-readme", "readme.md", "# Anything", "readme.md"},
		{"from-title", "note.md", "# My Note", "my-note.md"},
		{"same-slug", "my-note.md", "# My Note", "my-note.md"},
		{"no-title", "x.md", "body only", "x.md"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := decideFilenameFromContent(tt.current, []byte(tt.content))
			if got != tt.want {
				t.Fatalf("got %q want %q", got, tt.want)
			}
		})
	}
}

func TestHtmlOutNameFor(t *testing.T) {
	chdirTemp(t)
	// With no index.md, readme.md -> index.html
	if got := htmlOutNameFor("readme.md"); got != "index.html" {
		t.Fatalf("got %q", got)
	}
	// Create index.md, then readme.md -> readme.html
	if err := os.WriteFile("index.md", nil, 0644); err != nil {
		t.Fatal(err)
	}
	if got := htmlOutNameFor("readme.md"); got != "readme.html" {
		t.Fatalf("got %q", got)
	}
	if got := htmlOutNameFor("note.md"); got != "note.html" {
		t.Fatalf("got %q", got)
	}
}

func TestFileExistsLower(t *testing.T) {
	chdirTemp(t)
	if err := os.WriteFile("Index.MD", nil, 0644); err != nil {
		t.Fatal(err)
	}
	if !fileExistsLower("index.md") {
		t.Fatalf("expected true")
	}
	if fileExistsLower("missing.md") {
		t.Fatalf("expected false")
	}
}

func TestFileExistsLower_ReadDirError(t *testing.T) {
	dir := chdirTemp(t)
	// Remove read permission from current directory to force ReadDir error
	if err := os.Chmod(dir, 0300); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0700) })
	if fileExistsLower("anything.md") {
		t.Fatalf("expected false on error")
	}
}

func TestUniqueAvailableName(t *testing.T) {
	chdirTemp(t)
	if err := os.WriteFile("note.md", []byte("hi"), 0644); err != nil {
		t.Fatal(err)
	}
	got := uniqueAvailableName("note.md")
	if got != "note-1.md" {
		t.Fatalf("got %q", got)
	}
}

func TestCopyIncludesToDocs(t *testing.T) {
	chdirTemp(t)
	// Create tree _includes/a/b.txt
	if err := os.MkdirAll(filepath.Join("_includes", "a"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("_includes", "a", "b.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := copyIncludesToDocs("_includes", "docs"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join("docs", "a", "b.txt")); err != nil {
		t.Fatalf("copied file missing: %v", err)
	}
}

func TestCopyIncludesToDocs_NoSrcOrNotDir(t *testing.T) {
	chdirTemp(t)
	// No src directory -> no error
	if err := copyIncludesToDocs("_includes", "docs"); err != nil {
		t.Fatal(err)
	}
	// Create a file named _includes to simulate non-dir
	if err := os.WriteFile("_includes", []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := copyIncludesToDocs("_includes", "docs"); err != nil {
		t.Fatal(err)
	}
}

func TestCopyIncludesToDocs_DstCreationError(t *testing.T) {
	chdirTemp(t)
	if err := os.MkdirAll("_includes", 0755); err != nil {
		t.Fatal(err)
	}
	// Create a file named docs so MkdirAll(dstDir) fails
	if err := os.WriteFile("docs", []byte("not a dir"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := copyIncludesToDocs("_includes", "docs"); err == nil {
		t.Fatalf("expected error when dst exists as file")
	}
}

func TestCopyFile(t *testing.T) {
	chdirTemp(t)
	if err := os.WriteFile("in.txt", []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := copyFile("in.txt", "out.txt"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile("out.txt")
	if string(b) != "hello" {
		t.Fatalf("content = %q", string(b))
	}
}

func TestCopyFile_NoSrcError(t *testing.T) {
	chdirTemp(t)
	if err := copyFile("missing.txt", "out.txt"); err == nil {
		t.Fatalf("expected error for missing source")
	}
}

func TestCopyTree_SrcNotExist(t *testing.T) {
	chdirTemp(t)
	if err := copyTree("nope", "dst"); err == nil {
		t.Fatalf("expected error for missing src")
	}
}

func TestExportMarkdownTo_WithHeaderFooter(t *testing.T) {
	// Skip on Windows due to shell script execution differences
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows")
	}
	chdirTemp(t)
	// Fake cmark: shell script that outputs simple HTML
	script := filepath.Join(t.TempDir(), "cmark.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho '<p>Body</p>'\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("in.md", []byte("# T"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll("_includes", 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("_includes", "header.html"), []byte("<h>H</h>"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("_includes", "footer.html"), []byte("<f>F</f>"), 0644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join("docs", "out.html")
	if err := exportMarkdownTo(script, "in.md", out); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.ReplaceAll(string(b), "\n", "")
	if got != "<h>H</h><p>Body</p><f>F</f>" {
		t.Fatalf("got %q", got)
	}
}

func TestExportMarkdownTo_NonMarkdown(t *testing.T) {
	chdirTemp(t)
	if err := exportMarkdownTo("/bin/echo", "in.txt", filepath.Join("docs", "out.html")); err != nil {
		t.Fatalf("expected nil for non-md, got %v", err)
	}
	if _, err := os.Stat(filepath.Join("docs", "out.html")); !os.IsNotExist(err) {
		t.Fatalf("output should not exist for non-md")
	}
}

func TestExportMarkdownTo_HeaderOnlyAndFooterOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows")
	}
	chdirTemp(t)
	script := filepath.Join(t.TempDir(), "cmark.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho '<p>Body</p>'\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("in.md", []byte("# T"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll("_includes", 0755); err != nil {
		t.Fatal(err)
	}
	// Header only
	if err := os.WriteFile(filepath.Join("_includes", "header.html"), []byte("<h>H</h>"), 0644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join("docs", "out1.html")
	if err := exportMarkdownTo(script, "in.md", out); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(out)
	if strings.ReplaceAll(string(b), "\n", "") != "<h>H</h><p>Body</p>" {
		t.Fatalf("unexpected output: %q", string(b))
	}
	// Footer only
	_ = os.Remove(filepath.Join("_includes", "header.html"))
	if err := os.WriteFile(filepath.Join("_includes", "footer.html"), []byte("<f>F</f>"), 0644); err != nil {
		t.Fatal(err)
	}
	out2 := filepath.Join("docs", "out2.html")
	if err := exportMarkdownTo(script, "in.md", out2); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(out2)
	if strings.ReplaceAll(string(b), "\n", "") != "<p>Body</p><f>F</f>" {
		t.Fatalf("unexpected output: %q", string(b))
	}
}

func TestHandleLockUnlockAndValidation(t *testing.T) {
	chdirTemp(t)
	// Reset global locks for isolation
	locks = make(map[string]lockInfo)

	// Acquire new lock
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/lock?file=note.md", nil)
	handleLock(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("lock status = %d", rr.Code)
	}
	tok := rr.Header().Get("X-Lock")
	if tok == "" {
		t.Fatalf("missing token")
	}
	// Refresh with same token
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/lock?file=note.md", nil)
	req.Header.Set("X-Lock", tok)
	handleLock(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("refresh status = %d", rr.Code)
	}
	// hasValidLock true
	if !hasValidLock("note.md", tok) {
		t.Fatalf("expected valid lock")
	}
	// Wrong token rejected
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/unlock?file=note.md", nil)
	req.Header.Set("X-Lock", "bad")
	handleUnlock(rr, req)
	if rr.Code != http.StatusLocked {
		t.Fatalf("unlock wrong token status = %d", rr.Code)
	}
	// Correct token unlocks
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/unlock?file=note.md", nil)
	req.Header.Set("X-Lock", tok)
	handleUnlock(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("unlock status = %d", rr.Code)
	}
	// Expiry behavior: insert expired lock and ensure invalid + removed
	locks["x.md"] = lockInfo{token: "t", expires: time.Now().Add(-time.Second)}
	if hasValidLock("x.md", "t") {
		t.Fatalf("expired should be invalid")
	}
	if _, ok := locks["x.md"]; ok {
		t.Fatalf("expired lock should be deleted")
	}
}

func TestHandleLock_ErrorsAndContention(t *testing.T) {
	chdirTemp(t)
	locks = make(map[string]lockInfo)
	// Method not allowed
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/lock?file=a.md", nil)
	handleLock(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("got %d", rr.Code)
	}
	// Contention: second locker should get 423
	// Contention: second locker should get 423
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/lock?file=a.md", nil)
	handleLock(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("got %d", rr.Code)
	}
	// Verify lock exists
	if _, ok := locks["a.md"]; !ok {
		t.Fatalf("expected lock to exist")
	}
	// Second request without token should be treated as another client
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/lock?file=a.md", nil)
	handleLock(rr, req)
	if rr.Code != http.StatusLocked {
		t.Fatalf("expect locked, got %d", rr.Code)
	}
}

func TestHandleLock_UseProvidedToken(t *testing.T) {
	chdirTemp(t)
	locks = make(map[string]lockInfo)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/lock?file=a.md", nil)
	req.Header.Set("X-Lock", "provided")
	handleLock(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("got %d", rr.Code)
	}
	if tok := rr.Header().Get("X-Lock"); tok != "provided" {
		t.Fatalf("expected provided token, got %q", tok)
	}
}

func TestHasValidLock_NoLock(t *testing.T) {
	chdirTemp(t)
	locks = make(map[string]lockInfo)
	if hasValidLock("missing.md", "tok") {
		t.Fatalf("expected false for missing lock")
	}
}

func TestHandleUnlock_MethodNotAllowed(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/unlock?file=a.md", nil)
	handleUnlock(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("got %d", rr.Code)
	}
}

func TestTransferLock(t *testing.T) {
	chdirTemp(t)
	locks = make(map[string]lockInfo)
	tok := "abc"
	locks["old.md"] = lockInfo{token: tok, expires: time.Now().Add(time.Second)}
	transferLock("old.md", "new.md", tok)
	if _, ok := locks["old.md"]; ok {
		t.Fatalf("old should be removed")
	}
	if li, ok := locks["new.md"]; !ok || li.token != tok {
		t.Fatalf("new should exist with token")
	}
}

func TestTransferLock_NoopOnMismatch(t *testing.T) {
	chdirTemp(t)
	locks = make(map[string]lockInfo)
	locks["old.md"] = lockInfo{token: "tok", expires: time.Now().Add(time.Second)}
	transferLock("old.md", "new.md", "wrong")
	if _, ok := locks["new.md"]; ok {
		t.Fatalf("should not transfer with wrong token")
	}
	if _, ok := locks["old.md"]; !ok {
		t.Fatalf("old should still exist")
	}
}

func TestNewToken(t *testing.T) {
	a := newToken()
	b := newToken()
	if a == b || a == "" || len(a) != 32 {
		t.Fatalf("tokens should be unique hex length 32")
	}
}

func TestHandleNew(t *testing.T) {
	chdirTemp(t)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/new", nil)
	handleNew(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d", rr.Code)
	}
	if _, err := os.Stat("untitled.md"); err != nil {
		t.Fatalf("file missing: %v", err)
	}
	// Second call should be 200
	rr = httptest.NewRecorder()
	handleNew(rr, req)
	if rr.Code != http.StatusCreated {
		// unique name still considered created
		t.Fatalf("status = %d", rr.Code)
	}
}

func TestHandleNew_CreateError(t *testing.T) {
	chdirTemp(t)
	if err := os.Mkdir("ro", 0500); err != nil {
		t.Fatal(err)
	}
	cwd, _ := os.Getwd()
	if err := os.Chdir("ro"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd); _ = os.Chmod("ro", 0755) })
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/new", nil)
	handleNew(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
}

func TestOpenLastMarkdown(t *testing.T) {
	chdirTemp(t)
	// No .md -> creates untitled.md
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/open", nil)
	openLastMarkdown(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	if _, err := os.Stat("untitled.md"); err != nil {
		t.Fatalf("untitled.md not created: %v", err)
	}
	if got := rr.Header().Get("X-Filename"); got != "untitled.md" {
		t.Fatalf("X-Filename = %q", got)
	}
	// Create files and pick most recent
	if err := os.WriteFile("a.md", []byte("a"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("b.md", []byte("b"), 0644); err != nil {
		t.Fatal(err)
	}
	// Make a.md newer
	now := time.Now()
	if err := os.Chtimes("a.md", now, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	rr = httptest.NewRecorder()
	openLastMarkdown(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	if got := rr.Header().Get("X-Filename"); got != "a.md" {
		t.Fatalf("expected a.md, got %q", got)
	}
}

func TestCreateFileIfNotExists(t *testing.T) {
	chdirTemp(t)
	p, created, err := createFileIfNotExists("x.txt")
	if err != nil || !created || p != "x.txt" {
		t.Fatalf("unexpected: %v %v %q", err, created, p)
	}
	p, created, err = createFileIfNotExists("x.txt")
	if err != nil || created != false || p != "x.txt" {
		t.Fatalf("unexpected second: %v %v %q", err, created, p)
	}
}

func TestFindLastMarkdownFile(t *testing.T) {
	chdirTemp(t)
	if p, err := findLastMarkdownFile("."); err != nil || p != "" {
		t.Fatalf("expected none, got %q err=%v", p, err)
	}
	if err := os.WriteFile("a.md", []byte("a"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("b.md", []byte("b"), 0644); err != nil {
		t.Fatal(err)
	}
	// Make b.md older than a.md
	past := time.Now().Add(-time.Hour)
	if err := os.Chtimes("b.md", past, past); err != nil {
		t.Fatal(err)
	}
	p, err := findLastMarkdownFile(".")
	if err != nil || filepath.Base(p) != "a.md" {
		t.Fatalf("expected a.md, got %q err=%v", p, err)
	}
}

func TestFindLastMarkdownFile_SkipsDirAndNonMD(t *testing.T) {
	chdirTemp(t)
	if err := os.Mkdir("sub", 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("c.txt", []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("a.md", []byte("a"), 0644); err != nil {
		t.Fatal(err)
	}
	p, err := findLastMarkdownFile(".")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if filepath.Base(p) != "a.md" {
		t.Fatalf("expected a.md, got %s", p)
	}
}

func TestOpenLastMarkdown_ReadDirError(t *testing.T) {
	// Remove read permission on cwd so os.ReadDir fails inside findLastMarkdownFile
	dir := chdirTemp(t)
	// Remove read bit; keep execute so we can chmod back
	if err := os.Chmod(dir, 0300); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0700) })
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/open", nil)
	openLastMarkdown(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
}

func TestHandleSave_WithRenameAndLock(t *testing.T) {
	chdirTemp(t)
	locks = make(map[string]lockInfo)
	// Initial file name note.md, will rename to my-note.md via title
	// Acquire lock
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/lock?file=note.md", nil)
	handleLock(rr, req)
	tok := rr.Header().Get("X-Lock")
	if tok == "" {
		t.Fatal("no token")
	}

	body := strings.NewReader("# My Note\nbody")
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/save?file=note.md", io.NopCloser(body))
	req.Header.Set("X-Lock", tok)
	handleSave(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("save status = %d", rr.Code)
	}
	newName := rr.Header().Get("X-Filename")
	if newName != "my-note.md" {
		t.Fatalf("expected rename to my-note.md, got %q", newName)
	}
	if _, err := os.Stat("my-note.md"); err != nil {
		t.Fatalf("renamed file missing: %v", err)
	}
	if _, err := os.Stat("note.md"); !os.IsNotExist(err) {
		t.Fatalf("old file should be removed")
	}
}

func TestHandleSave_RequiresLock(t *testing.T) {
	chdirTemp(t)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/save?file=note.md", strings.NewReader("hi"))
	handleSave(rr, req)
	if rr.Code != http.StatusLocked {
		t.Fatalf("expected 423, got %d", rr.Code)
	}
}

type errBody struct{}

func (errBody) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
func (errBody) Close() error             { return nil }

func TestHandleSave_BodyReadErrorAndMethodAndTraversal(t *testing.T) {
	chdirTemp(t)
	// Method not allowed
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/save", nil)
	handleSave(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("got %d", rr.Code)
	}
	// Body read error
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/save", errBody{})
	handleSave(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("got %d", rr.Code)
	}
	// Path traversal rejected
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/save?file=../evil.md", strings.NewReader("hi"))
	handleSave(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("got %d", rr.Code)
	}
}

func TestHandleSave_HeaderFallbackAndDefaultAndExport(t *testing.T) {
	// Also covers export branch and removal of old exported HTML on rename
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows")
	}
	chdirTemp(t)
	locks = make(map[string]lockInfo)

	// Header fallback, no rename, default content
	// Acquire lock for header name
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/lock?file=from-header.md", nil)
	handleLock(rr, req)
	tok := rr.Header().Get("X-Lock")

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/save", strings.NewReader("body without title"))
	req.Header.Set("X-Filename", "from-header.md")
	req.Header.Set("X-Lock", tok)
	handleSave(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("got %d", rr.Code)
	}
	if _, err := os.Stat("from-header.md"); err != nil {
		t.Fatalf("file missing: %v", err)
	}

	// Default to index.md when no file and no header (also test export)
	// Need a lock for index.md
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/lock?file=index.md", nil)
	handleLock(rr, req)
	tok = rr.Header().Get("X-Lock")

	// Fake cmark to enable export branch
	script := filepath.Join(t.TempDir(), "cmark.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho '<p>Body</p>'\n"), 0755); err != nil {
		t.Fatal(err)
	}
	cmarkPath = script
	t.Cleanup(func() { cmarkPath = "" })

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/save", strings.NewReader("no title"))
	req.Header.Set("X-Lock", tok)
	handleSave(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("got %d", rr.Code)
	}
	if _, err := os.Stat("index.md"); err != nil {
		t.Fatalf("index.md missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join("docs", "index.html")); err != nil {
		t.Fatalf("export missing: %v", err)
	}

	// Rename removes old export
	// Pre-create docs/note.html
	if err := os.MkdirAll("docs", 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("docs", "note.html"), []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	// Lock for note.md
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/lock?file=note.md", nil)
	handleLock(rr, req)
	tok = rr.Header().Get("X-Lock")
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/save?file=note.md", strings.NewReader("# Renamed Title"))
	req.Header.Set("X-Lock", tok)
	handleSave(rr, req)
	if _, err := os.Stat(filepath.Join("docs", "note.html")); !os.IsNotExist(err) {
		t.Fatalf("old export should be removed")
	}
}

func TestHandleLoadIndex_PermissionDenied(t *testing.T) {
	chdirTemp(t)
	if err := os.WriteFile("index.md", []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod("index.md", 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod("index.md", 0644) })
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/index", nil)
	handleLoadIndex(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
}

func TestOpenLastMarkdown_WriteError(t *testing.T) {
	chdirTemp(t)
	if err := os.WriteFile("a.md", []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}
	ew := &errWriter{}
	req := httptest.NewRequest(http.MethodGet, "/open", nil)
	openLastMarkdown(ew, req)
	if ew.code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", ew.code)
	}
}

func TestOpenLastMarkdown_ReadError(t *testing.T) {
	chdirTemp(t)
	if err := os.WriteFile("a.md", []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod("a.md", 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod("a.md", 0644) })
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/open", nil)
	openLastMarkdown(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
}

func TestCopyTree_PermissionError(t *testing.T) {
	chdirTemp(t)
	if err := os.MkdirAll("src", 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("src", "f.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll("dst", 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod("dst", 0755) })
	if err := copyTree("src", "dst"); err == nil {
		t.Fatalf("expected error due to dst permissions")
	}
}

func TestCopyTree_DstSubdirCreationError(t *testing.T) {
	chdirTemp(t)
	// src has a subdir "a"
	if err := os.MkdirAll(filepath.Join("src", "a"), 0755); err != nil {
		t.Fatal(err)
	}
	// dst has a file named "a" which will block MkdirAll("dst/a")
	if err := os.MkdirAll("dst", 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("dst", "a"), []byte("file"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := copyTree("src", "dst"); err == nil {
		t.Fatalf("expected error creating dst subdir")
	}
}

func TestCreateFileIfNotExists_PermissionDenied(t *testing.T) {
	chdirTemp(t)
	if err := os.Mkdir("ro", 0500); err != nil {
		t.Fatal(err)
	}
	cwd, _ := os.Getwd()
	if err := os.Chdir("ro"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd); _ = os.Chmod("ro", 0755) })
	if _, _, err := createFileIfNotExists("x.txt"); err == nil {
		t.Fatalf("expected permission error")
	}
}

func TestExportMarkdownTo_CommandError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows")
	}
	chdirTemp(t)
	// Script exits with non-zero
	script := filepath.Join(t.TempDir(), "cmark_fail.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 1\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("in.md", []byte("# T"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := exportMarkdownTo(script, "in.md", filepath.Join("docs", "out.html")); err == nil {
		t.Fatalf("expected error from cmark")
	}
}

func TestExportMarkdownTo_MkdirAllError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows")
	}
	chdirTemp(t)
	// Create a file named docs to make MkdirAll fail
	if err := os.WriteFile("docs", []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(t.TempDir(), "cmark.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho '<p>Body</p>'\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("in.md", []byte("# T"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := exportMarkdownTo(script, "in.md", filepath.Join("docs", "out.html")); err == nil {
		t.Fatalf("expected mkdir error")
	}
}

func TestHandleSave_UniqueAvailableName(t *testing.T) {
	chdirTemp(t)
	locks = make(map[string]lockInfo)
	// Pre-create my-note.md to force unique name
	if err := os.WriteFile("my-note.md", []byte("existing"), 0644); err != nil {
		t.Fatal(err)
	}
	// Lock note.md
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/lock?file=note.md", nil)
	handleLock(rr, req)
	tok := rr.Header().Get("X-Lock")
	// Save content that renames to my-note.md
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/save?file=note.md", strings.NewReader("# My Note"))
	req.Header.Set("X-Lock", tok)
	handleSave(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("got %d", rr.Code)
	}
	if got := rr.Header().Get("X-Filename"); got != "my-note-1.md" {
		t.Fatalf("expected my-note-1.md, got %q", got)
	}
	if _, err := os.Stat("my-note-1.md"); err != nil {
		t.Fatalf("new file missing: %v", err)
	}
}

func TestCleanAndExportAll(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows")
	}
	chdirTemp(t)
	// Pre-existing docs content should be removed
	if err := os.MkdirAll("docs", 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("docs", "junk.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	// Create markdown files, including readme.md special-case
	if err := os.WriteFile("readme.md", []byte("# Readme"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("note.md", []byte("# Note"), 0644); err != nil {
		t.Fatal(err)
	}
	// Fake cmark
	script := filepath.Join(t.TempDir(), "cmark.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho '<p>Body</p>'\n"), 0755); err != nil {
		t.Fatal(err)
	}
	cmarkPath = script
	t.Cleanup(func() { cmarkPath = "" })
	if err := cleanAndExportAll("docs"); err != nil {
		t.Fatal(err)
	}
	// Junk removed
	if _, err := os.Stat(filepath.Join("docs", "junk.txt")); !os.IsNotExist(err) {
		t.Fatalf("junk should be removed")
	}
	// Exports created
	if _, err := os.Stat(filepath.Join("docs", "note.html")); err != nil {
		t.Fatalf("note.html missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join("docs", "index.html")); err != nil {
		t.Fatalf("index.html missing: %v", err)
	}
}

func TestCleanAndExportAll_NoCmarkLeavesDocs(t *testing.T) {
	chdirTemp(t)
	// Create docs with a file that should remain if no cmark available
	if err := os.MkdirAll("docs", 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("docs", "keep.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	// Ensure cmarkPath is empty
	cmarkPath = ""
	if err := cleanAndExportAll("docs"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join("docs", "keep.txt")); err != nil {
		t.Fatalf("docs should be untouched when no cmark: %v", err)
	}
}
