// Package analyzer performs semantic analysis, symbol resolution,
// operator precedence folding, and type checking on ASTs.
package analyzer

import (
	"sort"

	"github.com/vertex-language/vsc/token"
)

// Scope represents a lexical scope holding symbols.
type Scope struct {
	parent   *Scope
	children []*Scope
	elems    map[string]Symbol
	pos      token.Pos
	end      token.Pos
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

// hoistInto moves everything declared in s into dst. A guard's
// conditions are read in a scope of their own so that its else block
// cannot see them; what they bound belongs to the enclosing scope
// once the guard has been passed.
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
