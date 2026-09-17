package importer

import "runtime"

// isWindows is the host, as a manifest's platform conditions name it.
func isWindows() bool { return runtime.GOOS == "windows" }
