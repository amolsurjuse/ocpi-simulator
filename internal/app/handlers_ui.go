package app

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func (a *App) serveUIIfEnabled(w http.ResponseWriter, r *http.Request, path string) bool {
	if !a.cfg.UIEnabled {
		return false
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}

	staticDir := strings.TrimSpace(a.uiStaticDir)
	if staticDir == "" {
		return false
	}

	relativePath := strings.Trim(path, "/")
	switch {
	case relativePath == "", relativePath == "ocpp-simulator-ui":
		return a.serveUIFile(w, r, staticDir, "index.html")
	case strings.HasPrefix(relativePath, "ocpp-simulator-ui/"):
		relativePath = strings.TrimPrefix(relativePath, "ocpp-simulator-ui/")
	}

	cleanPath := filepath.ToSlash(filepath.Clean("/" + relativePath))
	cleanPath = strings.TrimPrefix(cleanPath, "/")
	if cleanPath == "." {
		cleanPath = ""
	}

	if cleanPath != "" && a.serveUIFile(w, r, staticDir, cleanPath) {
		return true
	}

	// Keep missing static assets as 404s, but fall back to SPA entrypoint for route URLs.
	if filepath.Ext(cleanPath) != "" {
		http.NotFound(w, r)
		return true
	}

	return a.serveUIFile(w, r, staticDir, "index.html")
}

func (a *App) serveUIFile(w http.ResponseWriter, r *http.Request, staticDir, relativePath string) bool {
	target := filepath.Join(staticDir, filepath.FromSlash(relativePath))
	info, err := os.Stat(target)
	if err != nil || info.IsDir() {
		return false
	}

	http.ServeFile(w, r, target)
	return true
}
