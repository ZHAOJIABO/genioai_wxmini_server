package admin_ui

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed index.html css js
var staticFiles embed.FS

// Handler 返回管理后台静态文件的 HTTP handler
func Handler() http.Handler {
	fsys, _ := fs.Sub(staticFiles, ".")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 尝试提供静态文件，找不到则返回 index.html (SPA)
		path := r.URL.Path
		if path == "" || path == "/" {
			path = "index.html"
		}

		// 检查文件是否存在
		f, err := staticFiles.Open(path)
		if err != nil {
			// 文件不存在，返回 index.html（SPA 路由）
			indexContent, _ := staticFiles.ReadFile("index.html")
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write(indexContent)
			return
		}
		f.Close()

		http.FileServer(http.FS(fsys)).ServeHTTP(w, r)
	})
}
