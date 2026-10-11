// Package logs turns application output into bounded, deterministic summaries.
package logs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

type Level int

const (
	Unclassified Level = iota
	Trace
	Debug
	Info
	Warning
	Error
	Fatal
)

type ParserConfig struct {
	Format     string            `json:"format,omitempty" yaml:"format,omitempty"`
	LevelField string            `json:"levelField,omitempty" yaml:"levelField,omitempty"`
	Patterns   map[string]string `json:"patterns,omitempty" yaml:"patterns,omitempty"`
	LevelMap   map[string]string `json:"levelMap,omitempty" yaml:"levelMap,omitempty"`
}

type Classification struct {
	Level   Level
	Invalid bool
	Blank   bool
}

type pattern struct {
	level Level
	re    *regexp.Regexp
}

type Parser struct {
	config   ParserConfig
	patterns []pattern
}

var ansi = regexp.MustCompile("\x1b(?:\\[[0-?]*[ -/]*[@-~]|\\][^\x07\x1b]*(?:\x07|\x1b\\\\))")
var header = regexp.MustCompile(`(?i)^(?:(?:\[?\d{4}[-/]\d\d[-/]\d\d[T ][0-9:.+Z-]+\]?|\d\d:\d\d:\d\d(?:[.,]\d+)?)\s+)?\[?(TRACE|DEBUG|INFO|WARN(?:ING)?|ERROR|ERR|FATAL|CRITICAL|PANIC)\]?(?:\s|:|$)`)
var levelKey = regexp.MustCompile(`(?i)(?:^|\s)level=(?:"|')?(trace|debug|info|warn(?:ing)?|error|err|fatal|critical|panic)(?:"|')?(?:\s|$)`)

func NewParser(c ParserConfig) (*Parser, error) {
	if c.Format == "" {
		c.Format = "auto"
	}
	if c.Format != "auto" && c.Format != "text" && c.Format != "jsonl" {
		return nil, fmt.Errorf("log format must be auto, text, or jsonl")
	}
	if c.LevelField == "" {
		c.LevelField = "level"
	}
	if len(c.LevelField) > 256 || len(c.Patterns) > 16 || len(c.LevelMap) > 32 {
		return nil, fmt.Errorf("log parser exceeds configuration limits")
	}
	p := &Parser{config: c}
	for label, mapped := range c.LevelMap {
		if len(label) == 0 || len(label) > 64 || severity(mapped) == Unclassified {
			return nil, fmt.Errorf("levelMap requires bounded labels mapped to known severities")
		}
	}
	for key := range c.Patterns {
		if severity(key) == Unclassified {
			return nil, fmt.Errorf("unknown severity in log pattern")
		}
	}
	// Resolve overlapping custom rules deterministically, highest severity first.
	for _, name := range []string{"fatal", "critical", "panic", "error", "err", "warning", "warn", "info", "debug", "trace"} {
		if expression, ok := c.Patterns[name]; ok {
			if len(expression) == 0 || len(expression) > 1024 {
				return nil, fmt.Errorf("log pattern must contain 1–1024 bytes")
			}
			r, err := regexp.Compile(expression)
			if err != nil {
				return nil, fmt.Errorf("invalid log pattern for %s", name)
			}
			p.patterns = append(p.patterns, pattern{severity(name), r})
		}
	}
	for name := range c.Patterns {
		if name != strings.ToLower(name) {
			return nil, fmt.Errorf("log pattern severity keys must be lowercase")
		}
	}
	return p, nil
}

func severity(s string) Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "trace":
		return Trace
	case "debug":
		return Debug
	case "info":
		return Info
	case "warn", "warning":
		return Warning
	case "error", "err":
		return Error
	case "fatal", "critical", "panic":
		return Fatal
	}
	return Unclassified
}

// Classify never assigns severity from stdout/stderr identity. Unmatched text is
// valid but unclassified; malformed explicit JSON is a format failure.
func (p *Parser) Classify(raw []byte) Classification {
	if !utf8.Valid(raw) {
		return Classification{Invalid: true}
	}
	line := bytes.TrimSpace(ansi.ReplaceAll(raw, nil))
	if len(line) == 0 {
		return Classification{Blank: true}
	}
	if p.config.Format == "jsonl" || p.config.Format == "auto" && line[0] == '{' {
		var value any
		if err := json.Unmarshal(line, &value); err == nil {
			if _, object := value.(map[string]any); !object && p.config.Format == "jsonl" {
				return Classification{Invalid: true}
			}
			for _, key := range strings.Split(p.config.LevelField, ".") {
				object, ok := value.(map[string]any)
				if !ok {
					value = nil
					break
				}
				value = object[key]
			}
			var label string
			switch v := value.(type) {
			case string:
				label = v
			case float64:
				label = fmt.Sprint(v)
			}
			if mapped, ok := p.config.LevelMap[label]; ok {
				label = mapped
			}
			if label != "" {
				if level := severity(label); level != Unclassified {
					return Classification{Level: level}
				}
			}
			if p.config.Format == "jsonl" {
				return Classification{}
			}
		} else if p.config.Format == "jsonl" {
			return Classification{Invalid: true}
		}
	}
	for _, rule := range p.patterns {
		if rule.re.Match(line) {
			return Classification{Level: rule.level}
		}
	}
	if match := header.FindSubmatch(line); match != nil {
		return Classification{Level: severity(string(match[1]))}
	}
	// Ignore key-looking strings inside the usual logfmt message field.
	if i := bytes.Index(line, []byte("msg=")); i >= 0 {
		line = line[:i]
	}
	if match := levelKey.FindSubmatch(line); match != nil {
		return Classification{Level: severity(string(match[1]))}
	}
	return Classification{}
}
