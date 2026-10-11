package watch

// LogSource configures a live collector independently of its input transport.
type LogSource struct {
	Input       LogInput          `json:"input" yaml:"input"`
	Format      string            `json:"format,omitempty" yaml:"format,omitempty"`
	LevelField  string            `json:"levelField,omitempty" yaml:"levelField,omitempty"`
	Patterns    map[string]string `json:"patterns,omitempty" yaml:"patterns,omitempty"`
	TextProfile string            `json:"textProfile,omitempty" yaml:"textProfile,omitempty"`
	Framing     string            `json:"framing,omitempty" yaml:"framing,omitempty"`
	Multiline   *LogMultiline     `json:"multiline,omitempty" yaml:"multiline,omitempty"`
	Window      string            `json:"window,omitempty" yaml:"window,omitempty"`
	Retention   string            `json:"retention,omitempty" yaml:"retention,omitempty"`
	EmitEvery   string            `json:"emitEvery,omitempty" yaml:"emitEvery,omitempty"`
	MinEvents   int               `json:"minEvents,omitempty" yaml:"minEvents,omitempty"`
	MinCoverage float64           `json:"minCoverage,omitempty" yaml:"minCoverage,omitempty"`
}

type LogMultiline struct {
	Start      string `json:"start" yaml:"start"`
	Continue   string `json:"continue" yaml:"continue"`
	MaxLines   int    `json:"maxLines,omitempty" yaml:"maxLines,omitempty"`
	FlushAfter string `json:"flushAfter,omitempty" yaml:"flushAfter,omitempty"`
}

type LogInput struct {
	Type           string               `json:"type" yaml:"type"`
	Path           string               `json:"path,omitempty" yaml:"path,omitempty"`
	Start          string               `json:"start,omitempty" yaml:"start,omitempty"`
	Argv           []string             `json:"argv,omitempty" yaml:"argv,omitempty"`
	Directory      string               `json:"directory,omitempty" yaml:"directory,omitempty"`
	Env            map[string]SecretRef `json:"env,omitempty" yaml:"env,omitempty"`
	ReconnectEvery string               `json:"reconnectEvery,omitempty" yaml:"reconnectEvery,omitempty"`
}
