package mangle

import (
	"crypto/md5"
	"encoding/hex"
	"strconv"
	"strings"

	"github.com/vertex-language/vsc/types"
)

// A NominalKind is what a named type is, which the mangling spells as
// one letter at the end of the name.
type NominalKind byte

const (
	Struct NominalKind = 'V'
	Class  NominalKind = 'C'
	Enum   NominalKind = 'O'
	// There is no kind letter for a protocol. A protocol appears in a
	// mangling only as part of an existential, and the `_p` that
	// closes the list is what says the names in it were protocols:
	// swiftc writes `AA1P_p`, not `AA1PP_p`. See protocolName.
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

	// ModuleOf is the module a type was declared in, asked of every
	// nominal type a signature mentions.
	//
	// It is not always the declaration's own module, and a symbol
	// that assumes it is will not link. `Top.widen(_ m: Base.Metric)`
	// is written in Top and takes a type from Base, and swiftc
	// mangles the parameter with Base's name; spelling it Top.Metric
	// produces a symbol nothing defines.
	//
	// Optional: with no answer, or with none given, the declaration's
	// own module is used, which is right for a module compiled alone.
	ModuleOf func(types.Type) string
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
	// Discriminator is set for a declaration that is private to one
	// file, and tells two same-named private declarations in different
	// files apart. Discriminator computes one.
	Discriminator string
}

// MetadataAccessor is the symbol of the function that hands back a
// type's metadata.
//
// A class's allocating initializer takes the metatype as its self
// argument, and for a class that metatype is a real pointer rather
// than the nothing a struct's thin one is. Where it comes from is
// this function: swiftc's own client code calls
// `$s2cl7CounterCMa` with a request of zero and puts the answer in
// the self register.
//
// The name is the type's own mangling and then Ma, so the Decl says
// only where the type lives and what it is called -- Context carries
// the type, and everything else is unused.
// NominalType is a nominal type's own symbol, with no suffix on it:
// `$s4main4MineV` for a struct Mine in module main.
//
// Everything a type needs at run time is that name and a suffix --
// `Mn` for its descriptor, `Ma` for its accessor, `WV` for its value
// witness table, `N` for the metadata itself -- so this is what a
// consumer that needs several of them asks for once.
func NominalType(d Decl) (string, error) {
	m := &mangler{moduleName: d.Module, moduleOf: d.ModuleOf, subAt: -1}
	m.write("$s")
	if err := m.context(d); err != nil {
		return "", err
	}
	return string(m.buf), nil
}

// ModuleDescriptor is the symbol of the record that names a module:
// `$s4mainMXM`. Every type's descriptor points at its module's, and
// the module's points at nothing.
func ModuleDescriptor(module string) (string, error) {
	m := &mangler{moduleName: module, subAt: -1}
	m.write("$s")
	if err := m.module(module); err != nil {
		return "", err
	}
	m.write("MXM")
	return string(m.buf), nil
}

// A Conf names a conformance: which type satisfies which protocol,
// and which module said so.
//
// The protocol is written as its module and its name with no kind
// letter after it, which is the one place a nominal type is spelled
// that way -- swiftc writes `$s3App4MineV3Lib8MeasuredAAMc`, where
// `3Lib8Measured` is the protocol and the `AA` after it is the module
// the conformance is declared in, by back-reference.
type Conf struct {
	// Type is the conforming type: its module and its nesting.
	Type Decl
	// ProtocolModule and ProtocolName are the protocol.
	ProtocolModule, ProtocolName string
	// Module is where the conformance is declared, which is not
	// always either of the two above.
	Module string
}

// ProtocolDescriptor is the symbol of the record that describes a
// protocol: `$ss5ErrorMp` for Swift's Error. A conformance points at
// one, and the library that declared the protocol is what exports it.
func ProtocolDescriptor(module, name string) (string, error) {
	m := &mangler{moduleName: module, subAt: -1}
	m.write("$s")
	if err := m.module(module); err != nil {
		return "", err
	}
	if err := m.identifier(name); err != nil {
		return "", err
	}
	m.write("Mp")
	return string(m.buf), nil
}

// WitnessTable is the symbol of a conformance's table.
func WitnessTable(c Conf) (string, error) { return conformance(c, "WP") }

// ConformanceDescriptor is the symbol of the record that says what
// the table is a conformance of.
func ConformanceDescriptor(c Conf) (string, error) { return conformance(c, "Mc") }

func conformance(c Conf, suffix string) (string, error) {
	m := &mangler{moduleName: c.Type.Module, moduleOf: c.Type.ModuleOf, subAt: -1}
	m.write("$s")
	if err := m.context(c.Type); err != nil {
		return "", err
	}
	if err := m.module(c.ProtocolModule); err != nil {
		return "", err
	}
	if err := m.identifier(c.ProtocolName); err != nil {
		return "", err
	}
	m.remember("nominal:" + c.ProtocolModule + "." + c.ProtocolName)
	if err := m.module(c.Module); err != nil {
		return "", err
	}
	m.write(suffix)
	return string(m.buf), nil
}

func MetadataAccessor(d Decl) (string, error) {
	m := &mangler{moduleName: d.Module, moduleOf: d.ModuleOf, subAt: -1}
	m.write("$s")
	if err := m.context(d); err != nil {
		return "", err
	}
	m.write("Ma")
	return string(m.buf), nil
}

// Getter is the symbol a computed property's getter is given.
//
// A computed property is a function that looks like a field, and its
// symbol says so: the context, the property's name, the type it hands
// back, and `vg` -- v for a variable, g for its getter. swiftc writes
// `$s5Wider3VecV9magnitudes5Int32Vvg` for `Vec.magnitude`, and that
// name is what a client calls; there is no field to read.
//
// The type is taken from the signature's result, which is the
// property's type: a getter takes nothing but the receiver.
func Getter(d Decl) (string, error) { return accessor(d, "vg") }

// StaticGetter is the symbol of a computed property of the type
// rather than of an instance: the same name with Z after it.
func StaticGetter(d Decl) (string, error) { return accessor(d, "vgZ") }

// Addressor is the symbol of the function that hands back the address
// of a stored property of the type.
//
// A static stored property is not a field of anything, so there is no
// offset to read it at: Swift gives it storage of its own and an
// accessor that returns where that storage is, initialising it the
// first time if it has to. swiftc's own client code for `Vec.unit`
// calls `$s5Wider3VecV4units5Int32Vvau` and loads what comes back.
func Addressor(d Decl) (string, error) { return accessor(d, "vau") }

// accessor writes a property's symbol: the context, the name, the
// type it is, and the letters that say which accessor this is.
func accessor(d Decl, kind string) (string, error) {
	m := &mangler{moduleName: d.Module, moduleOf: d.ModuleOf, subAt: -1}
	m.write("$s")
	if err := m.context(d); err != nil {
		return "", err
	}
	if err := m.identifier(d.Name); err != nil {
		return "", err
	}
	if d.Signature == nil || d.Signature.Results == nil {
		return "", fail(ErrUnsupported, "a property accessor with no type")
	}
	if err := m.typ(d.Signature.Results); err != nil {
		return "", err
	}
	m.write(kind)
	return string(m.buf), nil
}

// Function is the symbol a function is given.
func Function(d Decl) (string, error) {
	m := &mangler{moduleName: d.Module, moduleOf: d.ModuleOf, subAt: -1}
	m.write("$s")
	if err := m.context(d); err != nil {
		return "", err
	}
	if err := m.identifier(d.Name); err != nil {
		return "", err
	}
	if d.Discriminator != "" {
		if err := m.identifier(d.Discriminator); err != nil {
			return "", err
		}
		m.write("LL")
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

// Initializer is the symbol an initializer is given.
//
// Two things differ from a function and both come from what an
// initializer is. It has no name -- `init` is a keyword and not an
// identifier -- so nothing is written between the type and the
// signature. And it ends in `fC` rather than `F`: the C is the
// allocating entry point, the one a call to `P(v: 3)` reaches. A
// class also has an initializing entry point spelled `fc`, which
// runs after the allocation; a struct needs no such split, having
// nothing to allocate.
//
// The signature's result is the type being made, which is what puts
// the substitution for it where swiftc puts one.
func Initializer(d Decl) (string, error) {
	m := &mangler{moduleName: d.Module, moduleOf: d.ModuleOf, subAt: -1}
	m.write("$s")
	if err := m.context(d); err != nil {
		return "", err
	}
	if d.Discriminator != "" {
		if err := m.identifier(d.Discriminator); err != nil {
			return "", err
		}
		m.write("LL")
	}
	if err := m.signature(d.Signature); err != nil {
		return "", err
	}
	// The `c` is the function type the signature just described;
	// swiftc writes an initializer's type out in full where a
	// method's is implied.
	m.write("cfC")
	return string(m.buf), nil
}

// A mangler is the string being built and the substitutions made so
// far. The two are one thing: an index is a position in what has
// already been written down.
type mangler struct {
	// words are the words of every identifier written so far, which a
	// later one refers back to instead of spelling again. See
	// words.go.
	words []string

	buf []byte
	// moduleOf is where a nominal type was declared, when the caller
	// can say. See Decl.ModuleOf.
	moduleOf func(types.Type) string

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

	// generics is the type parameter list a `x`, `q_` or `q0_` is an
	// index into: a generic parameter has no number of its own, and
	// which one it is is its place in the signature that declared it.
	generics []*types.TypeParam

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
	// The chain is remembered under its whole dotted spelling, not
	// under each name alone. A later mention of `Chart.Point` is a
	// back-reference to that one entry, and remembering it as `Point`
	// makes it a different entry -- so the numbering drifts and the
	// symbol comes out with a substitution nothing put there.
	chain := ""
	for _, n := range d.Context {
		if err := m.identifier(n.Name); err != nil {
			return err
		}
		m.writeByte(byte(n.Kind))
		if chain != "" {
			chain += "."
		}
		chain += n.Name
		m.remember("nominal:" + d.Module + "." + chain)
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
	key := identKey(name)
	if i, ok := m.lookup(key); ok {
		m.substitution(i)
		return nil
	}
	if err := m.rawIdentifier(name); err != nil {
		return err
	}
	m.rememberWords(name)
	m.remember(key)
	return nil
}

// identifier writes a name, numbering it. Every identifier is
// numbered, whether or not anything will refer to it.
//
// A name that repeats a word already written is written as a
// reference to it rather than spelled again -- see words.go, which is
// where the encoding and the reason for it are.
func (m *mangler) identifier(name string) error {
	// A name written before in full is referred back to whole, and
	// that beats spelling it out of its words: `fib` in module `fib`
	// is AA, the module's own entry, not a word substitution of it.
	// A module name and a declaration's name share one table, which
	// is what makes that entry findable from here.
	if i, ok := m.lookup(identKey(name)); ok {
		m.substitution(i)
		m.rememberWords(name)
		return nil
	}
	if !m.substitutedIdentifier(name) {
		if err := m.rawIdentifier(name); err != nil {
			return err
		}
	}
	m.rememberWords(name)
	m.remember(identKey(name))
	return nil
}

// identKey is how a name is filed in the substitution table. A module
// and a declaration are filed alike because swiftc refers to one from
// the other.
func identKey(name string) string { return "ident:" + name }

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
	if sig.Async {
		return fail(ErrUnsupported, "an async function")
	}
	// The parameters a `x` or `q_` in the types below refers to. Kept
	// on the mangler rather than passed down: a generic parameter may
	// appear anywhere in the result or in the arguments, and every one
	// of them means the same thing.
	prev := m.generics
	m.generics = sig.TypeParams
	defer func() { m.generics = prev }()

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
	// The generic signature, which comes after the parameters and
	// before the F: one parameter at depth zero is the default and is
	// written as `l` alone, and more than one says how many.
	//
	// swiftc's own symbols say it: `one<T>(T) -> Int32` is
	// `$s1m3oneys5Int32VxlF`, `two<T, U>` is `...x_q_tr0_lF` and
	// `three<A, B, C>` is `...x_q_q0_tr1_lF`. The number after the r
	// is a mangled index, where the first index is written as nothing
	// at all -- so a count of two is index one and reads `0_`.
	if n := len(sig.TypeParams); n > 0 {
		for _, tp := range sig.TypeParams {
			if tp != nil && len(tp.Constraints) > 0 {
				return fail(ErrUnsupported, "a constrained generic parameter")
			}
		}
		if n > 1 {
			m.writeByte('r')
			m.index(n - 1)
		}
		m.writeByte('l')
	}
	return nil
}

// paramIndex is a generic parameter's place in the signature that
// declared it, or -1 where no signature in scope declares it.
func (m *mangler) paramIndex(t *types.TypeParam) int {
	for i, p := range m.generics {
		if p == t {
			return i
		}
	}
	// A parameter matched by name rather than by identity: a
	// substituted signature is rebuilt, and the list on it may hold
	// different pointers for the same declaration.
	for i, p := range m.generics {
		if p != nil && t != nil && p.Name == t.Name {
			return i
		}
	}
	return -1
}

// index writes a mangled index: the first is nothing followed by an
// underscore, and every later one is the number before it.
func (m *mangler) index(n int) {
	if n > 0 {
		m.write(strconv.Itoa(n - 1))
	}
	m.writeByte('_')
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
	case len(ps) == 1 && !labelled && !isVoid(ps[0].Type) && !isTuple(ps[0].Type) &&
		!ps[0].Variadic:
		// A lone unlabelled parameter is written as its own type.
		// Three exceptions, and the first two are the same mistake: a
		// type that would be read back as the list rather than as
		// what is in it. Void collapses (()) to (), turning a
		// function of one argument into a function of none. A tuple
		// collapses ((Int, Int)) to (Int, Int), turning one argument
		// into two -- swiftc writes `AD_ADt_t` for a lone tuple
		// parameter, where the trailing `_t` is the list the tuple is
		// inside.
		//
		// The third is a variadic parameter, whose `d` belongs to the
		// list and not to the type: swiftc writes `ADd_t` for a lone
		// one, keeping the list so that the marker has something to
		// be part of.
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
	if err := m.typ(p.Type); err != nil {
		return err
	}
	// A variadic parameter is the element type with a marker after
	// it, not the array the callee sees: swiftc writes
	// `$s1V5totalys5Int32VADd_tF` for `total(_ xs: Int32...)`, where
	// AD is Int32 by substitution and d says the rest was a list.
	if p.Variadic {
		m.writeByte('d')
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

// --- private declarations ---

// A private declaration is file-local, so two files in one module may
// each declare one with the same name and they must not collide. Swift
// separates them by writing a discriminator after the name and marking
// the result LL, and this does the same.
//
// The value is the one deliberate difference from swiftc. Swift's own
// discriminator is a hash it computes its own way, and the string only
// has to be stable and distinct per file: a private symbol never
// leaves the object file it is in, so nothing outside can depend on
// which string it was. Matching Swift's exactly would be worth doing
// the day a private symbol has to be resolved across compilers, and
// that day has not come.
func Discriminator(path string) string {
	sum := md5.Sum([]byte(path))
	return "_" + strings.ToUpper(hex.EncodeToString(sum[:]))
}

// isTuple reports whether a type is a tuple of more than nothing.
// The empty tuple is Void and is handled as Void.
func isTuple(t types.Type) bool {
	if t == nil {
		return false
	}
	tu, ok := t.Underlying().(*types.Tuple)
	return ok && len(tu.Elements) > 0
}
