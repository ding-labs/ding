package control

import (
	"github.com/ding-labs/ding/internal/watchrun"
	"net/http"
)

// consoleRoutes is extended by the operational-read and tool phases.
func consoleRoutes(mux *http.ServeMux, app *watchrun.App, cfg ConsoleConfig) {}
