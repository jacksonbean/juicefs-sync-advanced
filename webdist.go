package main

import (
	"embed"
	"io/fs"
)

//go:embed web/dist
var webDist embed.FS

func webDistFS() fs.FS {
	sub, err := fs.Sub(webDist, "web/dist")
	if err != nil {
		return nil
	}
	return sub
}
