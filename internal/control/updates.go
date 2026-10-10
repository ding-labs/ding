package control

import (
	"errors"
	"net/http"
	"os"

	"github.com/ding-labs/ding/internal/install"
	"github.com/ding-labs/ding/internal/update"
)

type LocalUpdates struct {
	Settings   update.Settings     `json:"settings"`
	Check      *update.CheckStatus `json:"check,omitempty"`
	Configured bool                `json:"configured"`
	Owner      string              `json:"owner,omitempty"`
}

func updateRoutes(mux *http.ServeMux, cfg ConsoleConfig) {
	mux.HandleFunc("GET /v1/local/updates", func(w http.ResponseWriter, r *http.Request) {
		s, err := update.LoadSettings(cfg.StateDir)
		if err != nil {
			fail(w, 409, "update_settings_invalid", "Inspect update settings with ding update configure.")
			return
		}
		result := LocalUpdates{Settings: s, Configured: update.PublicKey != ""}
		if record, err := install.Load(cfg.StateDir); err == nil {
			result.Owner = record.Owner
			result.Configured = result.Configured && record.Channel != "development"
		} else {
			result.Configured = false
		}
		check, err := update.ReadCheck(cfg.StateDir)
		if err == nil {
			result.Check = &check
		} else if !errors.Is(err, os.ErrNotExist) {
			fail(w, 409, "update_status_invalid", "Inspect the private update-status.json file; cached update status is unavailable.")
			return
		}
		write(w, 200, result, nil)
	})
}
