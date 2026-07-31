package version

import (
	"fmt"
	"runtime"
)

var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

func UserAgent() string {
	return fmt.Sprintf("seli-api-sdk/%s (%s; %s)", Version, runtime.GOOS, runtime.GOARCH)
}
