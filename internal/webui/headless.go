//go:build !console

package webui

import "net/http"

func Handler() http.Handler { return nil }
