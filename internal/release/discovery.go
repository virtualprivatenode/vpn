// Package release owns VPN release discovery and verification policy.
package release

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/virtualprivatenode/vpn/internal/paths"
	"github.com/virtualprivatenode/vpn/internal/system"
)

type githubRelease struct {
	TagName string `json:"tag_name"`
}

const versionCacheMaxAge = 24 * time.Hour

// CheckLatestVersion returns a cached version for up to 24 hours or fetches it
// over Tor and may update the cache. Empty means unavailable; a returned version
// does not establish update eligibility or that it is newer than this binary.
func CheckLatestVersion() string {
	if cached := readVersionCache(); cached != "" {
		return cached
	}

	if _, err := exec.LookPath("torsocks"); err != nil {
		return ""
	}
	output, err := system.RunOutputWithTimeout(10*time.Second,
		"torsocks", "curl", "-sL",
		"https://api.github.com/repos/virtualprivatenode/vpn/releases/latest")
	if err != nil {
		return ""
	}

	var release githubRelease
	if err := json.Unmarshal([]byte(output), &release); err != nil {
		return ""
	}

	version := strings.TrimPrefix(release.TagName, "v")
	if version != "" {
		writeVersionCache(version)
	}
	return version
}

func readVersionCache() string {
	info, err := os.Stat(paths.VersionCacheFile)
	if err != nil {
		return ""
	}
	if time.Since(info.ModTime()) > versionCacheMaxAge {
		return ""
	}
	data, err := os.ReadFile(paths.VersionCacheFile)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func writeVersionCache(version string) {
	existing := readVersionCache()
	if existing == version {
		return
	}
	os.MkdirAll(paths.VersionCacheDir, 0750)
	os.WriteFile(paths.VersionCacheFile,
		[]byte(version), 0600)
}
