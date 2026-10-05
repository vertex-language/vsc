package analyzer

import (
	"sort"

	"github.com/vertex-language/vsc/token"
)

// Scope represents a lexical block mapping identifiers to declared symbols.
type Scope struct {
	parent   *Scope
	children []*Scope
	elems    map[string]Symbol
	pos      token.Pos
	end      token.Pos
	// members says the scope holds a type's members, whose names are
	// found as members rather than as declarations of the module.
	members bool
}

// NewScope creates a new Scope with parent and source extent [pos, end].
func NewScope(parent *Scope, pos, end token.Pos) *Scope {
	s := &Scope{
		parent: parent,
		elems:  make(map[string]Symbol),
		pos:    pos,
		end:    end,
	}
	if parent != nil {
		parent.children = append(parent.children, s)
	}
	return s
}

// Parent returns the enclosing scope, or nil for the root/universe scope.
func (s *Scope) Parent() *Scope { return s.parent }

// Children returns all child scopes directly enclosed by s.
func (s *Scope) Children() []*Scope { return s.children }

// Pos returns the start position of this scope in source.
func (s *Scope) Pos() token.Pos { return s.pos }

// End returns the end position of this scope in source.
func (s *Scope) End() token.Pos { return s.end }

// Len returns the number of symbols directly declared in this scope.
func (s *Scope) Len() int { return len(s.elems) }

// Names returns all symbol names directly declared in this scope in alphabetical order.
func (s *Scope) Names() []string {
	names := make([]string, 0, len(s.elems))
	for name := range s.elems {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Insert inserts symbol sym into this scope. If a symbol with the same name
// already exists in this scope, Insert leaves the scope unchanged and returns the existing symbol.
// Otherwise it inserts sym and returns nil.
func (s *Scope) Insert(sym Symbol) Symbol {
	name := sym.Name()
	if existing, ok := s.elems[name]; ok {
		return existing
	}
	s.elems[name] = sym
	return nil
}

// hoistInto moves all symbols declared in s into dst.
func (s *Scope) hoistInto(dst *Scope) {
	for name, sym := range s.elems {
		dst.elems[name] = sym
		delete(s.elems, name)
	}
}

// Lookup looks up name in this scope, or any enclosing outer scope.
// It returns nil if name is not found.
func (s *Scope) Lookup(name string) Symbol {
	_, sym := s.LookupParent(name)
	return sym
}

// LookupType looks up the nearest type named name, passing over anything
// else of that name on the way out.
//
// A type position names a type. A method called Size on a type whose scope
// is searched first does not hide the type Size declared further out, any
// more than it does in Swift: `func Size() -> Size` is an ordinary thing to
// write.
func (s *Scope) LookupType(name string) *TypeNameSymbol {
	for curr := s; curr != nil; curr = curr.parent {
		if tn, ok := curr.elems[name].(*TypeNameSymbol); ok {
			return tn
		}
	}
	return nil
}

// LookupLocal looks up name ONLY in this scope.
func (s *Scope) LookupLocal(name string) Symbol {
	return s.elems[name]
}

// LookupParent looks up name starting in this scope and ascending parent scopes.
// It returns the scope where the symbol was found and the symbol itself, or (nil, nil).
func (s *Scope) LookupParent(name string) (*Scope, Symbol) {
	for curr := s; curr != nil; curr = curr.parent {
		if sym, ok := curr.elems[name]; ok {
			return curr, sym
		}
	}
	return nil, nil
}

// Symbols returns all symbols declared directly in this scope, including function overloads.
func (s *Scope) Symbols() []Symbol {
	out := make([]Symbol, 0, len(s.elems))
	for _, sym := range s.elems {
		out = append(out, sym)
		if fn, ok := sym.(*FuncSymbol); ok {
			for _, over := range fn.Overloads() {
				if over != sym {
					out = append(out, over)
				}
			}
		}
	}
	return out
}
