package version

import (
	"fmt"
	"runtime"
	"strconv"
	"time"
)

var (
	Version        = "v0.0.0"
	BuildTime      = "1706890000"
	BuildGoVersion = "unknown"
)

func FormatedVersion() string {
	return fmt.Sprintf("%s-%s %s %s-%s", Version, parsedBuildTime().Format(time.DateOnly), BuildGoVersion, runtime.GOOS, runtime.GOARCH)
}

func parsedBuildTime() time.Time {
	t, err := strconv.ParseInt(BuildTime, 10, 64)
	if err != nil {
		panic(err)
	}

	return time.Unix(t, 0)
}
