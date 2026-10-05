// Package build registers container backends for targets linked by this package.
package build

import (
	_ "github.com/vertex-language/macho/arm64"
	_ "github.com/vertex-language/pe/x64"
)
