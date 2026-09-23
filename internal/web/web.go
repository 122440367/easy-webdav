package web

import (
	"embed"
	"html/template"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed dist
var assets embed.FS

// Handler serves the embedded SPA and static assets. baseURL is the external
// mount prefix, such as /panel/ or /.
func Handler(baseURL string) http.Handler {
	baseURL = normalizeBase(baseURL)
	root, err := fs.Sub(assets, "dist")
	if err != nil {
		panic(err)
	}
	files := http.FileServer(http.FS(root))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if name != "" && name != "." {
			if f, openErr := root.Open(name); openErr == nil {
				_ = f.Close()
				files.ServeHTTP(w, r)
				return
			}
		}
		serveIndex(w, baseURL)
	})
}

func normalizeBase(baseURL string) string {
	baseURL = "/" + strings.Trim(baseURL, "/")
	if baseURL != "/" {
		baseURL += "/"
	}
	return baseURL
}

func serveIndex(w http.ResponseWriter, baseURL string) {
	data, err := fs.ReadFile(assets, "dist/index.html")
	if err != nil {
		http.Error(w, "frontend unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	content := string(data)
	base := template.HTMLEscapeString(baseURL)
	if strings.Contains(content, "<base ") {
		content = strings.Replace(content, "<base href=\"./\">", "<base href=\""+base+"\">", 1)
	} else {
		content = strings.Replace(content, "<head>", "<head><base href=\""+base+"\">", 1)
	}
	_, _ = w.Write([]byte(content))
}
