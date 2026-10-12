// Package integrationpolicy shares integration limits across HTTP and MCP.
package integrationpolicy

type Profile struct {
	Execution        string `json:"execution"`
	Workspace        string `json:"workspace,omitempty"`
	WorkspaceName    string `json:"workspaceName,omitempty"`
	MaxManifestBytes int    `json:"maxManifestBytes"`
	MaxFixtureBytes  int    `json:"maxFixtureBytes"`
	MaxRequestBytes  int    `json:"maxRequestBytes"`
}

const (
	CloudManifestBytes = 64 << 10
	CloudFixtureBytes  = 64 << 10
	// Fits both fields even with six-byte JSON escaping, plus the MCP envelope.
	// Individual field limits still apply before parsing or evaluation.
	CloudRequestBytes = 1 << 20
)

func Local() Profile {
	return Profile{Execution: "local", MaxManifestBytes: 1 << 20, MaxFixtureBytes: 8 << 20, MaxRequestBytes: 12 << 20}
}

func Hosted(workspace, name string, shared bool) Profile {
	p := Local()
	p.Execution, p.Workspace, p.WorkspaceName = "shared", workspace, name
	if !shared {
		p.Execution = "cloud"
		p.MaxManifestBytes, p.MaxFixtureBytes, p.MaxRequestBytes = CloudManifestBytes, CloudFixtureBytes, CloudRequestBytes
	}
	return p
}
