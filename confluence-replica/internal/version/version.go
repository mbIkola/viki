package version

import "strings"

var (
	// Version is injected at build time via -ldflags.
	Version = "dev"
	// Commit is injected at build time via -ldflags.
	Commit = "none"
	// BuildDate is injected at build time via -ldflags.
	BuildDate = "unknown"
)

// MCPVersion returns semver-ish value for MCP implementation metadata.
// For namespaced tags like "confluence-replica/v0.3.0" it returns "0.3.0".
func MCPVersion() string {
	v := strings.TrimSpace(Version)
	if v == "" || v == "dev" {
		return "dev"
	}
	if i := strings.LastIndex(v, "/"); i >= 0 && i+1 < len(v) {
		v = v[i+1:]
	}
	v = strings.TrimPrefix(v, "v")
	if v == "" {
		return "dev"
	}
	return v
}
