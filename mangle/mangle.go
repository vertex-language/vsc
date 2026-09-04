package mangle

import (
	"strconv"

	"github.com/vertex-language/vsc/types"
)

// A NominalKind is what a named type is, which the mangling spells as
// one letter at the end of the name.
type NominalKind byte

const (
	Struct NominalKind = 'V'
	Class  NominalKind = 'C'
	Enum   NominalKind = 'O'
)

// A Nominal is one type in the chain a declaration is nested inside.
type Nominal struct {
	Name string
	Kind NominalKind
}

// A Decl is a declaration to be given a symbol: where it lives, what
// it is called, and what it is.
type Decl struct {
	// Module is the module the declaration was written in.
	Module string
	// Context is the types it is nested inside, outermost first. A
	// method on a class has that class here; a free function has
	// nothing.
	Context []Nominal
	// Name is the declaration's own name.
	Name string
	// Signature is the function's type.
	Signature *types.Signature
	// Static says the function belongs to the type rather than to an
	// instance, which the mangling spells with an extra Z.
	Static bool
}

// Function is the symbol a function is given.
func Function(d Decl) (string, error) {
	m := &mangler{moduleName: d.Module, subAt: -1}
	m.write("$s")
	if err := m.context(d); err != nil {
		return "", err
	}
	if err := m.identifier(d.Name); err != nil {
		return "", err
	}
	if err := m.signature(d.Signature); err != nil {
		return "", err
	}
	m.writeByte('F')
	if d.Static {
		// A static function is a function that then says so. The Z
		// comes after the F, not before it.
		m.writeByte('Z')
	}
	return string(m.buf), nil
}

// A mangler is the string being built and the substitutions made so
// far. The two are one thing: an index is a position in what has
// already been written down.
type mangler struct {
	buf []byte
	// moduleName is the module being compiled, which a type declared
	// in it is written against.
	moduleName string

	// A run of the same standard substitution is written once with a
	// count -- SiSi is S2i -- so the tail of the buffer has to stay
	// rewritable. run records where the current run began, which
	// letter it is of, and how long it has got.
	runAt    int
	runCode  byte
	runCount int

	// A run of adjacent back-references merges the same way, so where
	// one began has to be remembered too. subAt is -1 when there is no
	// run in progress.
	subAt  int
	subLen int
	subRun []int
	// subs is what a demangler would have built, in order. Nothing is
	// ever removed, and an entry is added even where nothing could
	// refer to it, because an index counts positions rather than
	// candidates.
	subs []string
}

// remember adds an entry to the substitution table and returns its
// index.
func (m *mangler) remember(key string) {
	m.subs = append(m.subs, key)
}

// lookup finds a previously spelled thing, or reports that it is new.
func (m *mangler) lookup(key string) (int, bool) {
	for i, s := range m.subs {
		if s == key {
			return i, true
		}
	}
	return 0, false
}

// substitution writes a back-reference to one earlier thing.
//
// References that end up next to each other are written as one `A`,
// and there are two ways they fold. The same index twice in a row
// carries a count, so `ACAC` is `A2C`. Two different indices merge
// into one run, with a lowercase letter for every entry but the last,
// so `AFAD` is `AfD`. Both can happen at once, which is why the run is
// kept as a list of indices and written out again from scratch each
// time one is added to it.
func (m *mangler) substitution(n int) {
	if m.subAt >= 0 && m.subAt+m.subLen == len(m.buf) {
		m.subRun = append(m.subRun, n)
		m.buf = m.buf[:m.subAt]
	} else {
		m.endRun()
		m.subAt = len(m.buf)
		m.subRun = append(m.subRun[:0], n)
	}
	m.writeRun()
	m.subLen = len(m.buf) - m.subAt
}

// writeRun renders the whole run of back-references.
func (m *mangler) writeRun() {
	m.buf = append(m.buf, 'A')
	for i := 0; i < len(m.subRun); {
		// How many times this index repeats from here.
		j := i
		for j < len(m.subRun) && m.subRun[j] == m.subRun[i] {
			j++
		}
		if j-i > 1 {
			m.buf = append(m.buf, strconv.Itoa(j-i)...)
		}
		m.buf = append(m.buf, m.letter(m.subRun[i], j == len(m.subRun))...)
		i = j
	}
}

// letter spells one index. Past twenty-six it takes a number as well.
// The case of the letter says whether the run ends here.
func (m *mangler) letter(n int, last bool) []byte {
	var b []byte
	if n >= 26 {
		b = append(b, strconv.Itoa(n-26)...)
	}
	c := byte('A' + n%26)
	if !last {
		c += 'a' - 'A'
	}
	return append(b, c)
}

// context writes the module and the types a declaration is nested
// inside.
func (m *mangler) context(d Decl) error {
	if err := m.module(d.Module); err != nil {
		return err
	}
	for _, n := range d.Context {
		if err := m.identifier(n.Name); err != nil {
			return err
		}
		m.writeByte(byte(n.Kind))
		m.remember("nominal:" + d.Module + "." + n.Name)
	}
	return nil
}

// module writes a module name, or a back-reference to it. The standard
// library is `s` and is never numbered.
func (m *mangler) module(name string) error {
	if name == "Swift" {
		m.writeByte('s')
		return nil
	}
	key := "module:" + name
	if i, ok := m.lookup(key); ok {
		m.substitution(i)
		return nil
	}
	if err := m.rawIdentifier(name); err != nil {
		return err
	}
	m.remember(key)
	return nil
}

// identifier writes a name, numbering it. Every identifier is
// numbered, whether or not anything will refer to it.
func (m *mangler) identifier(name string) error {
	if err := m.rawIdentifier(name); err != nil {
		return err
	}
	m.remember("ident:" + name)
	return nil
}

// rawIdentifier writes a name and numbers nothing: the length, then
// the name.
func (m *mangler) rawIdentifier(name string) error {
	if name == "" {
		return fail(ErrName, "empty")
	}
	for i := 0; i < len(name); i++ {
		if name[i] >= 0x80 {
			// Swift punycodes a name that is not ASCII and marks it
			// with a leading 00. Writing the marker without the
			// encoding would produce a symbol that demangles to
			// something else.
			return fail(ErrName, "a name that is not ASCII: "+name)
		}
	}
	m.write(strconv.Itoa(len(name)))
	m.write(name)
	return nil
}

// signature writes the labels, the result, and the parameters, in that
// order -- which is not the order they are read back in.
func (m *mangler) signature(sig *types.Signature) error {
	if sig == nil {
		return fail(ErrUnsupported, "a function with no signature")
	}
	if len(sig.TypeParams) != 0 {
		return fail(ErrUnsupported, "a generic function")
	}
	if sig.Async {
		return fail(ErrUnsupported, "an async function")
	}

	// The label list is written only where there is something to label.
	// All-unlabelled is one `y`, and otherwise every parameter says its
	// own label or `_` for not having one.
	if len(sig.Params) > 0 {
		if labelled(sig) {
			for _, p := range sig.Params {
				if p.Label == "" || p.Label == "_" {
					m.writeByte('_')
					continue
				}
				if err := m.identifier(p.Label); err != nil {
					return err
				}
			}
		} else {
			m.writeByte('y')
		}
	}

	if err := m.result(sig.Results); err != nil {
		return err
	}
	if err := m.params(sig.Params, labelled(sig)); err != nil {
		return err
	}
	if sig.Throws {
		m.writeByte('K')
	}
	return nil
}

// result writes what a function gives back. A function that gives back
// nothing is written as the empty list rather than as the empty tuple
// -- `y` and not `yt` -- which is the one place the two spellings of
// nothing are not interchangeable.
func (m *mangler) result(t types.Type) error {
	if isVoid(t) {
		m.writeByte('y')
		return nil
	}
	return m.typ(t)
}

func isVoid(t types.Type) bool {
	switch t := t.(type) {
	case nil:
		return true
	case *types.Basic:
		return t.Kind() == types.Void
	case *types.Tuple:
		return len(t.Elements) == 0
	}
	return false
}

func labelled(sig *types.Signature) bool {
	for _, p := range sig.Params {
		if p.Label != "" && p.Label != "_" {
			return true
		}
	}
	return false
}

// params writes the parameter list.
//
// It is a tuple of the parameter types, with the labels left out --
// they were written before the result. A tuple of one collapses into
// the type it holds, exactly as it does in the language, but only when
// that one parameter has no label: `(a: Int)` is not `Int`, so it
// keeps the tuple.
func (m *mangler) params(ps []*types.Param, labelled bool) error {
	switch {
	case len(ps) == 0:
		m.writeByte('y')
		return nil
	case len(ps) == 1 && !labelled && !isVoid(ps[0].Type):
		// A lone unlabelled parameter is written as its own type. The
		// exception is a parameter of type Void: collapsing (()) to ()
		// would turn a function of one argument into a function of
		// none, which is a different function.
		return m.param(ps[0])
	}
	// A list marks where it begins rather than separating what is in
	// it: the first element is followed by an underscore and the rest
	// simply follow, so (Int, Bool, Int) is Si_SbSit.
	for i, p := range ps {
		if err := m.param(p); err != nil {
			return err
		}
		if i == 0 {
			m.writeByte('_')
		}
	}
	m.writeByte('t')
	return nil
}

func (m *mangler) param(p *types.Param) error {
	if p.Variadic {
		return fail(ErrUnsupported, "a variadic parameter")
	}
	if err := m.typ(p.Type); err != nil {
		return err
	}
	// An inout parameter is an address, and the mangling says so after
	// the type rather than before it.
	if p.Ownership == types.InOut {
		m.writeByte('z')
	}
	return nil
}

// --- the buffer ---
//
// Writing goes through here rather than to a builder because one
// rewrite is needed: a run of the same standard substitution collapses
// into a single one with a count, and whether a run is still running
// is only known when the next thing is written.

func (m *mangler) write(s string) {
	m.endRun()
	m.buf = append(m.buf, s...)
}

func (m *mangler) writeByte(b byte) {
	m.endRun()
	m.buf = append(m.buf, b)
}

// std writes one of the standard substitutions -- Si, Sb, SS -- and
// joins it to the run before it where there is one. Two Ints in a row
// are S2i and three are S3i, which is what swiftc emits and so what
// this has to.
func (m *mangler) std(code byte) {
	if m.runCount > 0 && m.runCode == code && m.runAt+m.runLen() == len(m.buf) {
		m.runCount++
		m.buf = m.buf[:m.runAt]
		m.buf = append(m.buf, 'S')
		m.buf = append(m.buf, strconv.Itoa(m.runCount)...)
		m.buf = append(m.buf, code)
		return
	}
	m.endRun()
	m.runAt = len(m.buf)
	m.runCode = code
	m.runCount = 1
	m.buf = append(m.buf, 'S', code)
}

// runLen is how many bytes the run currently occupies.
func (m *mangler) runLen() int {
	if m.runCount == 1 {
		return 2 // S and the letter
	}
	return 2 + len(strconv.Itoa(m.runCount))
}

// endRun says that whatever comes next cannot join either run.
func (m *mangler) endRun() {
	m.runCount = 0
	m.subAt = -1
}
