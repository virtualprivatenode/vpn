package release

import (
	"fmt"
	"regexp"
	"strings"
)

// releaseVersion is the accepted shape for a release version.
// Anchored and strict: this is the single choke point between
// "string from the network" and "string in a release URL".
var releaseVersion = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)

// SameMajor checks the current self-update version shape and major-version
// policy for both the TUI and helper. It does not establish newer ordering
// or compatibility of an appliance transition.
func SameMajor(current, target string) (bool, error) {
	if !releaseVersion.MatchString(current) {
		return false, fmt.Errorf(
			"running version %q is not a release build — "+
				"self-update requires one", current)
	}
	if !releaseVersion.MatchString(target) {
		return false, fmt.Errorf(
			"%q is not a valid release version", target)
	}
	return strings.SplitN(current, ".", 2)[0] ==
		strings.SplitN(target, ".", 2)[0], nil
}
