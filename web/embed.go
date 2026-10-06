package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var assets embed.FS

func Files() fs.FS { f, _ := fs.Sub(assets, "dist"); return f }
