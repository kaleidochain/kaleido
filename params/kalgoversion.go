package params

import (
	"fmt"
)

const (
	KalgoVersionMajor = 1          // Major version component of the current kalgo release
	KalgoVersionMinor = 0          // Minor version component of the current kalgo release
	KalgoVersionPatch = 6         // Patch version component of the current kalgo release
	KalgoVersionMeta  = "unstable" // Version metadata to append to the version string
)

// KalgoVersion holds the textual version string.
var KalgoVersion = func() string {
	return fmt.Sprintf("%d.%d.%d", KalgoVersionMajor, KalgoVersionMinor, KalgoVersionPatch)
}()

// KalgoVersionWithMeta holds the textual version string including the metadata.
var KalgoVersionWithMeta = func() string {
	v := KalgoVersion
	if KalgoVersionMeta != "" {
		v += "-" + KalgoVersionMeta
	}
	return v
}()

// KalgoArchiveVersion holds the textual version string used for archives.
// e.g. "1.8.11-dea1ce05" for stable releases, or
//      "1.8.13-unstable-21c059b6" for unstable releases
func KalgoArchiveVersion(gitCommit string) string {
	vsn := KalgoVersion
	if KalgoVersionMeta != "stable" {
		vsn += "-" + KalgoVersionMeta
	}
	if len(gitCommit) >= 8 {
		vsn += "-" + gitCommit[:8]
	}
	return vsn
}

func KalgoVersionWithCommit(gitCommit string) string {
	vsn := KalgoVersionWithMeta
	if len(gitCommit) >= 8 {
		vsn += "-" + gitCommit[:8]
	}
	return vsn
}
