package releasebootstrap

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"github.com/ding-labs/ding/internal/update"
	"golang.org/x/mod/semver"
)

// Homebrew uses the same qualified native archives as the signed channel, so
// macOS keeps the signed helper adjacent to the real executable in libexec.
func Homebrew(m update.Manifest) (string, []byte, error) {
	if !semver.IsValid(m.Version) || semver.Canonical(m.Version) != m.Version || (m.Channel != "stable" && m.Channel != "preview") || (m.Channel == "stable" && semver.Prerelease(m.Version) != "") {
		return "", nil, fmt.Errorf("invalid Homebrew release")
	}
	name, class, conflict := "ding", "Ding", ""
	if m.Channel == "preview" {
		name, class, conflict = "ding-preview", "DingPreview", "ding"
	}
	platforms := map[string]update.Artifact{}
	for _, a := range m.Artifacts {
		if a.OS != "darwin" && a.OS != "linux" {
			continue
		}
		key := a.OS + "_" + a.Arch
		expected := "https://github.com/ding-labs/ding/releases/download/" + m.Version + "/ding_" + key + ".tar.gz"
		if a.URL != expected || len(a.SHA256) != 64 || strings.Trim(a.SHA256, "0123456789abcdef") != "" {
			return "", nil, fmt.Errorf("invalid Homebrew artifact")
		}
		platforms[key] = a
	}
	for _, key := range []string{"darwin_arm64", "darwin_amd64", "linux_arm64", "linux_amd64"} {
		if platforms[key].URL == "" {
			return "", nil, fmt.Errorf("missing Homebrew platform %s", key)
		}
	}
	data := struct {
		Class, Conflict, Version string
		Platforms                map[string]update.Artifact
		OS, Arch                 []string
	}{class, conflict, strings.TrimPrefix(m.Version, "v"), platforms, []string{"darwin", "linux"}, []string{"arm64", "amd64"}}
	t, err := template.New("formula").Parse(homebrewFormula)
	if err != nil {
		return "", nil, err
	}
	var result bytes.Buffer
	err = t.Execute(&result, data)
	return name + ".rb", result.Bytes(), err
}

const homebrewFormula = `# typed: false
# frozen_string_literal: true

# Generated from qualified native archives; publication requires release review.
class {{.Class}} < Formula
  desc "Persistent watches and durable alerts for developers and agents"
  homepage "https://ding.ing"
  version "{{.Version}}"
  license "Apache-2.0"
  depends_on macos: :ventura if OS.mac?

{{range $os := .OS}}  on_{{if eq $os "darwin"}}macos{{else}}linux{{end}} do
{{range $arch := $.Arch}}    on_{{if eq $arch "arm64"}}arm{{else}}intel{{end}} do
{{$artifact := index $.Platforms (printf "%s_%s" $os $arch)}}      url "{{$artifact.URL}}"
      sha256 "{{$artifact.SHA256}}"
    end
{{end}}  end
{{end}}
{{if .Conflict}}  conflicts_with "{{.Conflict}}", because: "both provide the ding executable"
{{end}}

  def install
    libexec.install "ding"
    libexec.install "DingNotifications.app" if OS.mac?
    bin.install_symlink libexec/"ding"
    generate_completions_from_executable(bin/"ding", "completion", shells: [:bash, :zsh, :fish])
  end

  def caveats
    <<~EOS
      Run ding setup to choose background startup and create a watch without an account.
      Ding manages its own user service; do not register a second brew service.
      Before upgrading, run ding service stop. After upgrading, run ding setup.
      Homebrew owns these binaries; use brew upgrade instead of ding update install.
    EOS
  end

  test do
    assert_match "{{.Version}}", shell_output("#{bin}/ding version")
    assert_path_exists libexec/"DingNotifications.app/Contents/MacOS/DingNotifications" if OS.mac?
    system bin/"ding", "demo"
  end
end
`
