package watchrun

import (
	"github.com/ding-labs/ding/internal/logs"
	"github.com/ding-labs/ding/internal/watch"
)

func LogFraming(c watch.LogSource) logs.FrameConfig {
	f := logs.FrameConfig{Mode: c.Framing}
	if m := c.Multiline; m != nil {
		f.Start, f.Continue, f.MaxLines, f.FlushAfter = m.Start, m.Continue, m.MaxLines, m.FlushAfter
	}
	return f
}
