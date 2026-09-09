package vil

import "github.com/vertex-language/vsc/types"

// A Stage says what has been proved about a module.
//
// Raw is what vil/gen emits: ownership is written down, nothing has
// been checked, mark_uninitialized is still there, and the program
// may yet be rejected. Canonical is what vil/pass produces and what
// lower consumes: definite initialization has run, the ownership
// rules hold, and ARC is in place.
type Stage string

const (
	StageRaw       Stage = "raw"
	StageCanonical Stage = "canonical"
	StageLowered   Stage = "lowered"
)

// A Module is a compilation unit's VIL: its functions, its globals,
// and the tables that say how dynamic dispatch and protocol
// conformance are resolved.
type Module struct {
	name    string
	stage   Stage
	funcs   []*Func
	byName  map[string]*Func
	globals []*Global
	vtables []*VTable
	witness []*WitnessTable
	imports []string
	// reqs is the order a protocol lists its requirements in, by
	// protocol name. See Requirements.
	reqs map[string][]string
	// metadata is what a type declared here needs to be named at run
	// time, by the type's own name. See TypeMetadata.
	metadata      map[string]TypeMetadata
	metadataOrder []string
}

// NewModule starts an empty module at the given stage.
func NewModule(name string, stage Stage) *Module {
	return &Module{name: name, stage: stage, byName: map[string]*Func{}}
}

func (m *Module) Name() string { return m.name }

// LookupSource finds a function by the name it has in the source
// rather than by its symbol.
func (m *Module) LookupSource(name string) *Func {
	for _, f := range m.funcs {
		if f.SourceName() == name {
			return f
		}
	}
	return nil
}
func (m *Module) Stage() Stage                   { return m.stage }
func (m *Module) Funcs() []*Func                 { return m.funcs }
func (m *Module) Globals() []*Global             { return m.globals }
func (m *Module) VTables() []*VTable             { return m.vtables }
func (m *Module) WitnessTables() []*WitnessTable { return m.witness }
func (m *Module) Imports() []string              { return m.imports }

// SetStage records that a pipeline has moved the module on. Nothing
// here checks that it was entitled to; vil/verify does.
func (m *Module) SetStage(s Stage) { m.stage = s }

// Import records a module this one refers to. SIL prints these at the
// top, and they are what a reader needs to resolve the names.
func (m *Module) Import(name string) {
	for _, im := range m.imports {
		if im == name {
			return
		}
	}
	m.imports = append(m.imports, name)
}

// Func declares a function, or returns the one already declared under
// that name. A function with no blocks is a declaration; give it a
// block and it becomes a definition.
func (m *Module) Func(name string) *Func {
	if f, ok := m.byName[name]; ok {
		return f
	}
	f := &Func{m: m, name: name, typ: &FuncType{Convention: Thin}}
	m.funcs = append(m.funcs, f)
	m.byName[name] = f
	return f
}

// Lookup finds a function by name, or returns nil.
func (m *Module) Lookup(name string) *Func { return m.byName[name] }

// Global declares a module-level variable.
func (m *Module) Global(name string, t Type, l Linkage) *Global {
	g := &Global{name: name, typ: t, linkage: l}
	m.globals = append(m.globals, g)
	return g
}

// VTable adds a class's dispatch table.
func (m *Module) VTable(class string) *VTable {
	t := &VTable{Class: class}
	m.vtables = append(m.vtables, t)
	return t
}

// A TypeMetadata is what a nominal type declared in this module needs
// in order to be named at run time.
//
// Only the names are here. What the metadata is made of -- the value
// witnesses, the sizes, the field offsets -- is layout, and layout is
// lower's; what a symbol is called is mangling, and mangling is this
// side's. So this carries the mangled name of the type and the plain
// names a descriptor spells out, and lower appends the suffixes:
// `Mn` for the descriptor, `Ma` for the accessor, `WV` for the value
// witness table, `Mf` for the record the metadata sits inside.
type TypeMetadata struct {
	// Type is the type's name in this module, which is how a witness
	// table and an existential name it.
	Type string
	// Mangled is the type's own symbol, without a suffix:
	// `$s4main4MineV`.
	Mangled string
	// Name is what the descriptor calls it, and Module is what the
	// module descriptor calls the module it is in.
	Name, Module string
	// Layout is the type itself, which is where its size, its
	// alignment and its field offsets come from.
	Layout types.Type
	// ModuleSym is the module descriptor's symbol.
	ModuleSym string
}

// Metadata records that a type declared here needs metadata emitted
// for it.
func (m *Module) Metadata(t TypeMetadata) {
	if t.Type == "" || t.Mangled == "" {
		return
	}
	if m.metadata == nil {
		m.metadata = map[string]TypeMetadata{}
	}
	if _, ok := m.metadata[t.Type]; ok {
		return
	}
	m.metadata[t.Type] = t
	m.metadataOrder = append(m.metadataOrder, t.Type)
}

// MetadataTypes is every type this module said it would need
// metadata for, in the order they were asked for.
func (m *Module) MetadataTypes() []TypeMetadata {
	out := make([]TypeMetadata, 0, len(m.metadata))
	for _, n := range m.metadataOrder {
		out = append(out, m.metadata[n])
	}
	return out
}

// MetadataFor is what a type needs to be named at run time, and
// whether anything asked for it.
func (m *Module) MetadataFor(typ string) (TypeMetadata, bool) {
	t, ok := m.metadata[typ]
	return t, ok
}

// Requirements records the order a protocol lists its requirements
// in, which is the order every table for it is laid out in.
//
// A conformance's table gives the order away only when the module
// holds one. A call through an existential whose protocol is
// imported has no table here to read it from -- the table is in the
// library -- so the order has to come from the protocol itself, and
// this is where the protocol says it.
func (m *Module) Requirements(proto string, names []string) {
	if proto == "" || len(names) == 0 {
		return
	}
	if m.reqs == nil {
		m.reqs = map[string][]string{}
	}
	if _, ok := m.reqs[proto]; ok {
		return
	}
	m.reqs[proto] = append([]string(nil), names...)
}

// RequirementsOf is the order a protocol lists its requirements in.
func (m *Module) RequirementsOf(proto string) ([]string, bool) {
	names, ok := m.reqs[proto]
	return names, ok
}

// Protocols is every protocol whose requirement order is recorded.
func (m *Module) Protocols() map[string][]string { return m.reqs }

// WitnessTable adds one conformance.
func (m *Module) WitnessTable(typ, proto, module string, l Linkage) *WitnessTable {
	t := &WitnessTable{Type: typ, Protocol: proto, Module: module, Linkage: l}
	m.witness = append(m.witness, t)
	return t
}

// Entry adds a row to a table.
func (t *VTable) Entry(member, impl string) *VTable {
	t.Entries = append(t.Entries, TableEntry{Member: member, Impl: impl})
	return t
}

// Entry adds a row to a witness table.
func (t *WitnessTable) Entry(member, impl string) *WitnessTable {
	t.Entries = append(t.Entries, TableEntry{Member: member, Impl: impl})
	return t
}
