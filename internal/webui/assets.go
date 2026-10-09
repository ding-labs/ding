//go:build console

package webui

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed dist/*
var assets embed.FS

func Handler() http.Handler {
	sub, err := fs.Sub(assets, "dist")
	if err != nil {
		panic(err)
	}
	return Serve(sub)
}
