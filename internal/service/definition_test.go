package service

import (
	"encoding/xml"
	"io"
	"strings"
	"testing"

	"github.com/ding-labs/ding/internal/install"
)

func record() install.Record {
	return install.Record{Schema: 1, Executable: "/test/path with spaces/ding", StateDir: "/test/state & data", Owner: "standalone", Version: "dev", Channel: "development", Digest: strings.Repeat("a", 64)}
}

func TestDefinitionsEscapePathsAndBoundRestarts(t *testing.T) {
	for _, platform := range []string{"darwin", "linux", "windows"} {
		t.Run(platform, func(t *testing.T) {
			d, err := DefinitionFor(platform, "/test/home", "/test/config", "S-1-5-21-test", record())
			if err != nil {
				t.Fatal(err)
			}
			if d.Startup != "login" || d.Name == "" {
				t.Fatal(d)
			}
			if platform == "linux" {
				if !strings.Contains(d.Content, `"/test/state & data"`) || !strings.Contains(d.Content, "RestartSec=10") {
					t.Fatal(d.Content)
				}
			} else {
				decoder := xml.NewDecoder(strings.NewReader(d.Content))
				for {
					_, err := decoder.Token()
					if err == io.EOF {
						break
					}
					if err != nil {
						t.Fatal(err)
					}
				}
				if !strings.Contains(d.Content, "&amp;") {
					t.Fatal("unescaped XML")
				}
			}
		})
	}
}

func TestServiceIdentityAndInjection(t *testing.T) {
	r := record()
	a, _ := DefinitionFor("linux", "/home/test", "/home/test/.config", "1", r)
	r.StateDir += "other"
	b, _ := DefinitionFor("linux", "/home/test", "/home/test/.config", "1", r)
	if a.Name == b.Name {
		t.Fatal("different state directories share service identity")
	}
	r.Executable += "\nExecStart=/bin/other"
	if _, err := DefinitionFor("linux", "/home/test", "/home/test/.config", "1", r); err == nil {
		t.Fatal("accepted newline injection")
	}
	if got := systemdQuote("/test/%h/$USER"); got != `"/test/%%h/$$USER"` {
		t.Fatal(got)
	}
	if got := windowsQuote(`a\"b\`); got != `"a\\\"b\\"` {
		t.Fatal(got)
	}
}
