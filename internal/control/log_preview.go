package control

import (
	"net/http"

	"github.com/ding-labs/ding/internal/logs"
	"github.com/ding-labs/ding/internal/watch"
	"github.com/ding-labs/ding/internal/watchrun"
)

type LogPreview struct {
	Total      int      `json:"total"`
	Classified int      `json:"classified"`
	Warnings   int      `json:"warnings"`
	Errors     int      `json:"errors"`
	Invalid    int      `json:"invalid"`
	Levels     []string `json:"levels"`
}

func logPreviewRoute(mux *http.ServeMux) {
	slots := make(chan struct{}, 2)
	mux.HandleFunc("POST /v1/tools/log-preview", func(w http.ResponseWriter, r *http.Request) {
		select {
		case slots <- struct{}{}:
			defer func() { <-slots }()
		default:
			fail(w, 429, "tool_busy", "log preview is busy")
			return
		}
		var input struct {
			Config watch.LogSource `json:"config"`
			Sample string          `json:"sample"`
		}
		if err := decodeBounded(w, r, &input, 128<<10); err != nil || len(input.Sample) > 64<<10 {
			fail(w, 400, "invalid_preview", "provide a log sample of at most 64 KiB")
			return
		}
		p, err := logs.NewParser(logs.ParserConfig{Format: input.Config.Format, LevelField: input.Config.LevelField, Patterns: input.Config.Patterns, LevelMap: input.Config.LevelMap})
		if err != nil {
			fail(w, 400, "invalid_parser", err.Error())
			return
		}
		out := LogPreview{Levels: []string{}}
		f, err := logs.NewConfiguredFramer(watchrun.LogFraming(input.Config), func(data []byte, invalid bool) {
			c := logs.Classification{Invalid: true}
			if !invalid {
				c = p.Classify(data)
			}
			if c.Blank {
				return
			}
			out.Total++
			if c.Level != logs.Unclassified {
				out.Classified++
			}
			if c.Level == logs.Warning {
				out.Warnings++
			}
			if c.Level == logs.Error || c.Level == logs.Fatal {
				out.Errors++
			}
			level := []string{"unclassified", "trace", "debug", "info", "warning", "error", "fatal"}[c.Level]
			if c.Invalid {
				out.Invalid++
				level = "invalid"
			}
			if len(out.Levels) < 100 {
				out.Levels = append(out.Levels, level)
			}
		})
		if err != nil {
			fail(w, 400, "invalid_framing", err.Error())
			return
		}
		f.Write([]byte(input.Sample))
		f.Close()
		write(w, 200, out, nil)
	})
}
