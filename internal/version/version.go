package version

import (
	"fmt"
	"runtime"
)

var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"

	// DefaultAPIURL is the API host this binary falls back to when neither
	// --api-url, SELI_API_URL, nor the active profile names one.
	//
	// It is empty in source on purpose: the host is configuration, never code,
	// and it will change. Builds inject it with
	// -ldflags "-X .../internal/version.DefaultAPIURL=<host>" from the
	// SELI_DEFAULT_API_URL environment variable (Makefile, .goreleaser.yml). A
	// bare `go build` therefore yields a binary that must be told the host.
	DefaultAPIURL = ""
)

func UserAgent() string {
	return fmt.Sprintf("seli-api-sdk/%s (%s; %s)", Version, runtime.GOOS, runtime.GOARCH)
}
