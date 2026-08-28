package version

import "os"

// value is injected at build time via
// -ldflags "-X github.com/javdet/nib/internal/version.value=v0.7.5".
var value = "dev"

// Version returns the running application version.
// APP_VERSION wins so local/dev containers can report a real value
// without a linker-injected build.
func Version() string {
	if v := os.Getenv("APP_VERSION"); v != "" {
		return v
	}
	return value
}
