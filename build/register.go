// The backend registrations.
//
// Each container repository keys its writers and linkers on
// (architecture, format) at init time, so the arm that reaches one
// has to be linked in. Importing the object writer alone is not
// enough and fails late: the linker reads the objects happily and
// then reports no backend for a target it could already parse.
//
// One import per target this package links, and no more — the whole
// point of build being its own module is that a program which only
// wants to typecheck something pays for none of it.
package build

import (
	_ "github.com/vertex-language/macho/arm64"
)
