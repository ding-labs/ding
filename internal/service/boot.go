package service

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

// BootTemplate prepares a reviewable administrator-owned registration. It never
// elevates, creates an account, changes power settings, or edits the user's job.
func BootTemplate(goos, executable, dir, user, group string) (Definition, error) {
	name := regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.@\\-]{0,127}$`)
	if !filepath.IsAbs(executable) || !filepath.IsAbs(dir) || !name.MatchString(user) || strings.EqualFold(user, "root") || strings.EqualFold(user, "system") {
		return Definition{}, fmt.Errorf("absolute paths and an explicit non-root service account are required")
	}
	if group == "" {
		group = user
	}
	if !name.MatchString(group) || strings.IndexFunc(executable+dir, unicode.IsControl) >= 0 {
		return Definition{}, fmt.Errorf("invalid service identity or path")
	}
	hash := sha256.Sum256([]byte(dir))
	id := fmt.Sprintf("%x", hash[:8])
	d := Definition{Startup: "boot"}
	args := []string{executable, "daemon", "--state-dir", dir, "--listen", "127.0.0.1:0", "--background-log"}
	switch goos {
	case "linux":
		d.Name = "ding-system-" + id + ".service"
		d.Path = "/etc/systemd/system/" + d.Name
		for i, arg := range args {
			args[i] = systemdQuote(arg)
		}
		d.Content = "[Unit]\nDescription=Ding unattended watches\nAfter=network-online.target\nWants=network-online.target\n[Service]\nType=simple\nUser=" + user + "\nGroup=" + group + "\nExecStart=" + strings.Join(args, " ") + "\nRestart=on-failure\nRestartSec=10\nTimeoutStopSec=45\nUMask=0077\nNoNewPrivileges=yes\n[Install]\nWantedBy=multi-user.target\n"
	case "darwin":
		d.Name = "ing.ding.system." + id
		d.Path = "/Library/LaunchDaemons/" + d.Name + ".plist"
		var values strings.Builder
		for _, arg := range args {
			values.WriteString("<string>" + xmlText(arg) + "</string>")
		}
		d.Content = `<?xml version="1.0" encoding="UTF-8"?><plist version="1.0"><dict><key>Label</key><string>` + d.Name + `</string><key>UserName</key><string>` + xmlText(user) + `</string><key>GroupName</key><string>` + xmlText(group) + `</string><key>ProgramArguments</key><array>` + values.String() + `</array><key>RunAtLoad</key><true/><key>KeepAlive</key><dict><key>SuccessfulExit</key><false/></dict><key>ThrottleInterval</key><integer>10</integer><key>ExitTimeOut</key><integer>45</integer><key>Umask</key><integer>63</integer><key>ProcessType</key><string>Background</string></dict></plist>` + "\n"
	case "windows":
		d.Name = "DingSystem-" + id
		d.Path = "administrator-reviewed PowerShell"
		q := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
		command := windowsQuote(executable) + " service host --name " + windowsQuote(d.Name) + " --state-dir " + windowsQuote(dir)
		d.Content = "$ErrorActionPreference = 'Stop'\n" +
			"if (Get-Service -Name " + q(d.Name) + " -ErrorAction SilentlyContinue) { throw 'Service already exists; inspect its owner before changing it.' }\n" +
			"if (Test-Path " + q(dir) + ") { throw 'Use a new private directory for this service identity; never share an active user installation.' }\n" +
			"$dingCredential = Get-Credential -UserName " + q(user) + " -Message 'Existing least-privileged Ding service account'\n" +
			"if (!$dingCredential) { throw 'Canceled' }\n" +
			"New-Item -ItemType Directory " + q(dir) + " | Out-Null\n" +
			"icacls " + q(dir) + " /inheritance:r /grant:r ($dingCredential.UserName + ':(OI)(CI)F') '*S-1-5-18:(OI)(CI)F' '*S-1-5-32-544:(OI)(CI)F'\nif ($LASTEXITCODE -ne 0) { throw 'Private ACL setup failed' }\n" +
			"icacls " + q(dir) + " /setowner $dingCredential.UserName\nif ($LASTEXITCODE -ne 0) { throw 'State ownership failed' }\n" +
			"New-Service -Name " + q(d.Name) + " -BinaryPathName " + q(command) + " -Credential $dingCredential -StartupType Automatic\n" +
			"Start-Service -Name " + q(d.Name) + "\n"
	default:
		return d, fmt.Errorf("use your platform supervisor")
	}
	return d, nil
}
