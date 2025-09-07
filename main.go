package main

import (
    "embed"
    "flag"
    "io"
    "io/fs"
    "log"
    "net/http"
    "os"
    "os/exec"
    "path/filepath"
    "strings"
    "time"
)

//go:embed _includes/*
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

	log.Printf("Serving embedded UI on http://%s\n", *addr)
	if err := http.ListenAndServe(*addr, nil); err != nil {
		log.Fatal(err)
	}
}

func rootHandler() http.Handler {
    sub, err := fs.Sub(embeddedIncludes, "_includes")
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
	f, err := os.Open(indexPath)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "index.md not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Filename", filepath.Base(indexPath))
	if _, err := io.Copy(w, f); err != nil {
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
    if err := os.WriteFile(name, data, 0644); err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    // Trigger export after save if available/enabled for this file only
    if cmarkPath != "" {
        if err := exportMarkdownFile(cmarkPath, name); err != nil {
            log.Printf("export error for %s: %v", name, err)
        }
    }
    w.WriteHeader(http.StatusNoContent)
}

var cmarkPath string // discovered at startup if available

// exportMarkdownFile converts a single Markdown file to HTML in ./docs using
// cmark-gfm. The output filename matches the input basename with .html ext.
func exportMarkdownFile(cmark, src string) error {
    if !strings.EqualFold(filepath.Ext(src), ".md") {
        return nil
    }
    if err := os.MkdirAll("docs", 0755); err != nil {
        return err
    }
    base := filepath.Base(src)
    outPath := filepath.Join("docs", strings.TrimSuffix(base, filepath.Ext(base))+".html")
    cmd := exec.Command(cmark, src)
    out, err := cmd.Output()
    if err != nil {
        return err
    }
    return os.WriteFile(outPath, out, 0644)
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

	f, err := os.Open(file)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer f.Close()

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Filename", filepath.Base(file))
	if _, err := io.Copy(w, f); err != nil {
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
