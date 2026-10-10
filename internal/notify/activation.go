package notify

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
)

type Target struct {
	StateDir string `json:"state"`
	WatchID  string `json:"watch"`
}

var watchIDPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,127}$`)

func (t Target) valid() bool {
	return filepath.IsAbs(t.StateDir) && len(t.StateDir) <= 4096 &&
		!strings.ContainsAny(t.StateDir, "\x00\r\n") && (t.WatchID == "" || watchIDPattern.MatchString(t.WatchID))
}

// ActivationURL contains routing data only. The installed handler validates the
// target and obtains a new authenticated, single-use browser handoff on click.
// URL-safe base64 avoids command-line quoting ambiguities in Windows protocols.
func ActivationURL(t Target) string {
	if !t.valid() {
		return ""
	}
	data, _ := json.Marshal(t)
	return "ding-watch://watch/" + base64.RawURLEncoding.EncodeToString(data)
}

func ParseActivation(raw string) (Target, error) {
	var target Target
	u, err := url.Parse(raw)
	if err != nil || len(raw) > 8192 || u.Scheme != "ding-watch" || u.Host != "watch" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
		return target, fmt.Errorf("invalid Ding notification target")
	}
	data, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(u.Path, "/"))
	if err != nil {
		return target, fmt.Errorf("invalid Ding notification target")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&target); err != nil || !target.valid() || ActivationURL(target) != raw {
		return Target{}, fmt.Errorf("invalid Ding notification target")
	}
	return target, nil
}
