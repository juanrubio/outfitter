package engine

import (
	"os"
	"runtime"
	"strings"
)

// DetectPlatformNotice returns a short "<distro> <version> · <arch>"
// label for the host, read from /etc/os-release.
func DetectPlatformNotice() string {
	data, _ := os.ReadFile("/etc/os-release")
	return PlatformNoticeFromOSRelease(data, runtime.GOARCH)
}

// PlatformNoticeFromOSRelease builds the platform label from os-release
// content. It uses NAME + VERSION_ID (e.g. "Ubuntu 26.04 · amd64") and
// falls back to "Linux · <arch>" when the content is missing or
// unrecognizable.
func PlatformNoticeFromOSRelease(data []byte, goarch string) string {
	fields := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		fields[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), `"`)
	}

	name := fields["NAME"]
	if name == "" {
		name = "Linux"
	}
	notice := name
	if version := fields["VERSION_ID"]; version != "" {
		notice += " " + version
	}
	if goarch == "" {
		goarch = runtime.GOARCH
	}
	return notice + " · " + goarch
}
