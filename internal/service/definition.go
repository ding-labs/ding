// Package service manages only Ding-owned operating system registrations.
package service

import (
	"bytes"
	"crypto/sha256"
	"encoding/xml"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"github.com/ding-labs/ding/internal/install"
)

type Definition struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Startup string `json:"startup"`
	Content string `json:"-"`
}

func DefinitionFor(goos, home, config, userID string, r install.Record) (Definition, error) {
	if err := r.Validate(); err != nil {
		return Definition{}, err
	}
	for _, value := range []string{r.Executable, r.StateDir, home, config, userID} {
		if strings.IndexFunc(value, unicode.IsControl) >= 0 {
			return Definition{}, fmt.Errorf("service paths cannot contain control characters")
		}
	}
	hash := sha256.Sum256([]byte(r.StateDir))
	id := fmt.Sprintf("%x", hash[:8])
	args := []string{r.Executable, "daemon", "--state-dir", r.StateDir, "--listen", "127.0.0.1:0", "--background-log"}
	d := Definition{Startup: "login"}
	switch goos {
	case "darwin":
		d.Name = "ing.ding.watch." + id
		d.Path = filepath.Join(home, "Library", "LaunchAgents", d.Name+".plist")
		var values strings.Builder
		for _, arg := range args {
			values.WriteString("<string>" + xmlText(arg) + "</string>\n")
		}
		d.Content = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Label</key><string>` + d.Name + `</string>
<key>ProgramArguments</key><array>` + values.String() + `</array>
<key>RunAtLoad</key><true/>
<key>KeepAlive</key><dict><key>SuccessfulExit</key><false/></dict>
<key>ThrottleInterval</key><integer>10</integer>
<key>ExitTimeOut</key><integer>30</integer>
<key>ProcessType</key><string>Background</string>
</dict></plist>
`
	case "linux":
		d.Name = "ding-watch-" + id + ".service"
		d.Path = filepath.Join(config, "systemd", "user", d.Name)
		for i, arg := range args {
			args[i] = systemdQuote(arg)
		}
		d.Content = "[Unit]\nDescription=Ding persistent watches\nStartLimitIntervalSec=60\nStartLimitBurst=5\n\n[Service]\nType=simple\nExecStart=" + strings.Join(args, " ") + "\nRestart=on-failure\nRestartSec=10\nTimeoutStopSec=30\nUMask=0077\n\n[Install]\nWantedBy=default.target\n"
	case "windows":
		if userID == "" {
			return Definition{}, fmt.Errorf("Windows service requires the current user identity")
		}
		d.Name = "Ding-" + id
		d.Path = filepath.Join(r.StateDir, "service-task.xml")
		for i, arg := range args {
			args[i] = windowsQuote(arg)
		}
		d.Content = `<?xml version="1.0" encoding="UTF-8"?>
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
<RegistrationInfo><Description>Ding managed local watch service</Description></RegistrationInfo>
<Triggers><LogonTrigger><Enabled>true</Enabled><UserId>` + xmlText(userID) + `</UserId></LogonTrigger></Triggers>
<Principals><Principal id="Author"><UserId>` + xmlText(userID) + `</UserId><LogonType>InteractiveToken</LogonType><RunLevel>LeastPrivilege</RunLevel></Principal></Principals>
<Settings><MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy><DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries><StopIfGoingOnBatteries>false</StopIfGoingOnBatteries><StartWhenAvailable>true</StartWhenAvailable><Enabled>true</Enabled><ExecutionTimeLimit>PT0S</ExecutionTimeLimit><RestartOnFailure><Interval>PT1M</Interval><Count>3</Count></RestartOnFailure></Settings>
<Actions Context="Author"><Exec><Command>` + xmlText(r.Executable) + `</Command><Arguments>` + xmlText(strings.Join(args[1:], " ")) + `</Arguments></Exec></Actions>
</Task>
`
	default:
		return Definition{}, fmt.Errorf("automatic startup is unsupported on %s; run ding daemon under your supervisor", goos)
	}
	return d, nil
}

func xmlText(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}
func systemdQuote(s string) string {
	return strconv.Quote(strings.NewReplacer("%", "%%", "$", "$$").Replace(s))
}

// Quote a single CommandLineToArgvW argument, including trailing backslashes.
func windowsQuote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	slashes := 0
	for _, ch := range s {
		if ch == '\\' {
			slashes++
			continue
		}
		if ch == '"' {
			b.WriteString(strings.Repeat("\\", slashes*2+1))
		} else {
			b.WriteString(strings.Repeat("\\", slashes))
		}
		slashes = 0
		b.WriteRune(ch)
	}
	b.WriteString(strings.Repeat("\\", slashes*2))
	b.WriteByte('"')
	return b.String()
}
