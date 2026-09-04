package vil

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
}

// NewModule starts an empty module at the given stage.
func NewModule(name string, stage Stage) *Module {
	return &Module{name: name, stage: stage, byName: map[string]*Func{}}
}

func (m *Module) Name() string                   { return m.name }
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
