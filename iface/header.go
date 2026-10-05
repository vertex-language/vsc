package iface

import (
	"bufio"
	"bytes"
	"strings"
)

// A Header contains metadata extracted from leading comments in an interface file.
type Header struct {
	// Module is the module's name, from whichever spelling the file
	// uses. It is empty when the header does not say, and then the
	// file's own name is the only thing left to go on.
	Module string

	// Resilient reports whether the library was built with library
	// evolution, which is a different ABI from the one this compiler
	// generates.
	Resilient bool

	// Flags is the whole flags line, so that a diagnostic can quote
	// what it read rather than assert a conclusion about it.
	Flags string
}

// Header line prefixes, in both spellings. swiftc writes the first
// pair; this compiler writes the second.
const (
	swiftFlags   = "// swift-module-flags:"
	vertexModule = "// vertex-module-name:"
)

// ReadHeader reads what an interface says about itself.
//
// Only the leading comments are read: the header ends at the first
// line that is not a comment and not blank, which in an interface is
// its first `import` or declaration.
func ReadHeader(src []byte) Header {
	var h Header
	sc := bufio.NewScanner(bytes.NewReader(src))
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "//") {
			break
		}
		switch {
		case strings.HasPrefix(line, vertexModule):
			h.Module = strings.TrimSpace(strings.TrimPrefix(line, vertexModule))
		case strings.HasPrefix(line, swiftFlags):
			h.Flags = strings.TrimSpace(strings.TrimPrefix(line, swiftFlags))
			h.readFlags(h.Flags)
		}
	}
	return h
}

// readFlags parses module name and library evolution flags from swift-module-flags.
func (h *Header) readFlags(line string) {
	words := strings.Fields(line)
	for i, w := range words {
		switch w {
		case "-enable-library-evolution":
			h.Resilient = true
		case "-module-name":
			if i+1 < len(words) && h.Module == "" {
				h.Module = words[i+1]
			}
		}
	}
}
