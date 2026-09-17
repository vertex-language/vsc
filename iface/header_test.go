package iface

import "testing"

// What an interface says about itself.
//
// The declarations below the header are the same whichever ABI a
// library was built for -- swiftc emits the same text either way --
// so these lines are the only thing that tells them apart. Getting
// this wrong is not a diagnostic but a bus error, so the cases below
// are real headers, copied from what swiftc wrote.

func TestReadHeader(t *testing.T) {
	for _, c := range []struct {
		name      string
		src       string
		module    string
		resilient bool
	}{
		{"swiftc, ordinary", `// swift-interface-format-version: 1.0
// swift-compiler-version: Apple Swift version 6.3.1
// swift-module-flags: -target arm64-apple-macosx26.0 -enable-objc-interop -module-name Geometry
import Swift
public func addAll(_ a: Swift.Int32) -> Swift.Int32
`, "Geometry", false},

		{"swiftc, library evolution", `// swift-interface-format-version: 1.0
// swift-module-flags: -target arm64-apple-macosx26.0 -enable-objc-interop -enable-library-evolution -module-name Lib
import Swift
`, "Lib", true},

		{"this compiler's own", `// vertex-interface-format-version: 1.0
// vertex-module-name: Chrono

public func epochYearUTC() -> Int32
`, "Chrono", false},

		{"no header at all", "public func f() -> Int32\n", "", false},

		// The flag is a word, not a substring: a module built in a
		// directory whose name contains it is not resilient.
		{"the flag's name inside a path", `// swift-module-flags: -target arm64-apple-macosx26.0 -I /src/enable-library-evolution/x -module-name M
`, "M", false},

		// The header stops where the declarations start, so a comment
		// further down cannot claim anything.
		{"a comment below the declarations", `// vertex-module-name: A
public func f() -> Int32
// swift-module-flags: -enable-library-evolution -module-name B
`, "A", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			h := ReadHeader([]byte(c.src))
			if h.Module != c.module {
				t.Errorf("module %q, want %q", h.Module, c.module)
			}
			if h.Resilient != c.resilient {
				t.Errorf("resilient %v, want %v", h.Resilient, c.resilient)
			}
		})
	}
}
