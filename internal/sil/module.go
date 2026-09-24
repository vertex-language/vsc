package sil

import "github.com/vertex-language/vsc/types"

// Stage represents the verification status and lowering phase of a SIL module.
type Stage string

const (
	StageRaw       Stage = "raw"
	StageCanonical Stage = "canonical"
	StageLowered   Stage = "lowered"
)

// Module represents a compiled SIL module containing functions, globals, and metadata tables.
type Module struct {
	name          string
	stage         Stage
	funcs         []*Func
	byName        map[string]*Func
	globals       []*Global
	vtables       []*VTable
	witness       []*WitnessTable
	imports       []string
	reqs          map[string][]string // protocol name -> requirement order
	metadata      map[string]TypeMetadata
	metadataOrder []string
	// protocols are the descriptors of the protocols this module declares.
	protocols []string
}

// NewModule starts an empty module at the given stage.
func NewModule(name string, stage Stage) *Module {
	return &Module{name: name, stage: stage, byName: map[string]*Func{}}
}

func (m *Module) Name() string { return m.name }

// LookupSource finds a function by its unmangled source name.
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

// SetStage updates the module lowering stage.
func (m *Module) SetStage(s Stage) { m.stage = s }

// Import records a module dependency.
func (m *Module) Import(name string) {
	for _, im := range m.imports {
		if im == name {
			return
		}
	}
	m.imports = append(m.imports, name)
}

// Func declares or retrieves a function by symbol name.
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
// VTableNamed is the table for a class, or nil when there is none yet.
func (m *Module) VTableNamed(class string) *VTable {
	for _, t := range m.vtables {
		if t.Class == class {
			return t
		}
	}
	return nil
}

func (m *Module) VTable(class string) *VTable {
	t := &VTable{Class: class}
	m.vtables = append(m.vtables, t)
	return t
}

// TypeMetadata describes runtime metadata emission requirements for a nominal type.
type TypeMetadata struct {
	Type      string // Type name in this module.
	Mangled   string // Mangled symbol name without suffix.
	Name      string // Unmangled name.
	Module    string // Module name.
	Layout    types.Type
	ModuleSym string
	// Public is whether the type is part of this module's API. Another
	// module compiled against this one calls the accessor by name, so a
	// public type's has to be a symbol this one exports.
	Public bool
	// Imported is a type another module declared and emitted the record
	// for: this module refers to its full record, Mangled+"Mf", and emits
	// nothing.
	Imported bool
	// Protocols is, for an existential's record, the descriptor symbol of
	// each protocol it stands for, in order.
	Protocols []string
}

// StructuralKey returns a key for structural composite types (e.g. Array, Optional).
func StructuralKey(t types.Type) string { return "structural:" + t.String() }

// Metadata registers that a declared type requires runtime metadata emission.
func (m *Module) Metadata(t TypeMetadata) {
	if t.Type == "" || t.Mangled == "" {
		return
	}
	if m.metadata == nil {
		m.metadata = map[string]TypeMetadata{}
	}
	if was, ok := m.metadata[t.Type]; ok {
		if t.Public && !was.Public {
			was.Public = true
			m.metadata[t.Type] = was
		}
		return
	}
	m.metadata[t.Type] = t
	m.metadataOrder = append(m.metadataOrder, t.Type)
}

// MetadataTypes returns all registered type metadata in registration order.
func (m *Module) MetadataTypes() []TypeMetadata {
	out := make([]TypeMetadata, 0, len(m.metadata))
	for _, n := range m.metadataOrder {
		out = append(out, m.metadata[n])
	}
	return out
}

// MetadataFor returns registered metadata for a type name, if any.
func (m *Module) MetadataFor(typ string) (TypeMetadata, bool) {
	t, ok := m.metadata[typ]
	return t, ok
}

// Requirements records the requirement ordering for a protocol witness table layout.
func (m *Module) Requirements(proto string, names []string) {
	// A protocol with no requirements -- Error -- is recorded too: its
	// tables are the descriptor row and nothing else.
	if proto == "" {
		return
	}
	if m.reqs == nil {
		m.reqs = map[string][]string{}
	}
	if _, ok := m.reqs[proto]; ok {
		return
	}
	m.reqs[proto] = append([]string{}, names...)
}

// RequirementsOf returns the requirement ordering for a protocol.
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

// ProtocolDescriptor records that the module declares a protocol, whose
// descriptor it defines under this symbol.
func (m *Module) ProtocolDescriptor(symbol string) {
	for _, have := range m.protocols {
		if have == symbol {
			return
		}
	}
	m.protocols = append(m.protocols, symbol)
}

// ProtocolDescriptors is every protocol descriptor the module defines.
func (m *Module) ProtocolDescriptors() []string { return m.protocols }
