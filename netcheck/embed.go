package netcheck

import (
	"embed"
	"io/fs"
)

//go:embed ui/*
var assets embed.FS

func Files() fs.FS { f, _ := fs.Sub(assets, "ui"); return f }
