package host

import "strings"

// ValidV3OnionHostname checks the syntax of a native Tor v3 hostname.
func ValidV3OnionHostname(hostname string) bool {
	const suffix = ".onion"
	if len(hostname) != 56+len(suffix) ||
		!strings.HasSuffix(hostname, suffix) {
		return false
	}
	for _, c := range strings.TrimSuffix(hostname, suffix) {
		if (c < 'a' || c > 'z') && (c < '2' || c > '7') {
			return false
		}
	}
	return true
}
