package adminweb

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:static
var static embed.FS

func (h *Handler) assets(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" && r.Method != "HEAD" {
		http.Error(w, "method", 405)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/admin/")
	if path == "" {
		path = "admin.html"
	}
	if path != "admin.html" && !strings.HasPrefix(path, "assets/") {
		http.NotFound(w, r)
		return
	}
	sub, _ := fs.Sub(static, "static")
	copy := r.Clone(r.Context())
	url := *r.URL
	url.Path = "/" + path
	copy.URL = &url
	http.FileServer(http.FS(sub)).ServeHTTP(w, copy)
}
