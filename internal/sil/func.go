package sil

import "github.com/vertex-language/vsc/types"

// Linkage specifies symbol visibility in SIL.
type Linkage string

const (
	Public         Linkage = ""
	PackageLinkage Linkage = "package"
	Hidden         Linkage = "hidden"
	Private        Linkage = "private"
	Shared         Linkage = "shared"
	PublicExternal Linkage = "public_external"
	HiddenExternal Linkage = "hidden_external"
)

// Func represents a SIL function or external declaration.
type Func struct {
	m       *Module
	name    string
	source  string
	linkage Linkage
	typ     *FuncType
	attrs   []string
	blocks  []*Block
	nextID  int
}

// AttrSwift marks a foreign Swift function declaration.
const AttrSwift = "swift_foreign"

func (f *Func) Module() *Module { return f.m }
func (f *Func) Name() string    { return f.name }

// SourceName returns the unmangled source name for diagnostics.
func (f *Func) SourceName() string {
	if f.source == "" {
		return f.name
	}
	return f.source
}

func (f *Func) SetSourceName(s string) *Func { f.source = s; return f }
func (f *Func) Linkage() Linkage             { return f.linkage }
func (f *Func) Type() *FuncType              { return f.typ }
func (f *Func) Blocks() []*Block             { return f.blocks }
func (f *Func) Attrs() []string              { return f.attrs }

// IsDeclaration reports whether the function has a body here.
func (f *Func) IsDeclaration() bool { return len(f.blocks) == 0 }

// SetLinkage sets how visible the function is.
func (f *Func) SetLinkage(l Linkage) *Func { f.linkage = l; return f }

// SetAttr adds an attribute modifier (e.g. ossa, transparent, thunk).
func (f *Func) SetAttr(name string) *Func {
	for _, a := range f.attrs {
		if a == name {
			return f
		}
	}
	f.attrs = append(f.attrs, name)
	return f
}

// HasAttr reports whether the attribute was set.
func (f *Func) HasAttr(name string) bool {
	for _, a := range f.attrs {
		if a == name {
			return true
		}
	}
	return false
}

// OSSA reports whether the function is in ownership SSA form.
func (f *Func) OSSA() bool { return f.HasAttr("ossa") }

// Param appends a parameter to the function signature and entry block.
func (f *Func) Param(t Type, conv ParamConvention) *Value {
	f.typ.Params = append(f.typ.Params, Param{Type: t, Convention: conv})
	return f.Entry().Arg(t, ownershipOf(conv))
}

// ownershipOf maps parameter convention to callee value ownership.
func ownershipOf(c ParamConvention) Ownership {
	switch c {
	case ParamOwned, ParamIn:
		return Owned
	case ParamGuaranteed, ParamInGuaranteed:
		return Guaranteed
	}
	return None
}

// SetResult gives the function one result.
func (f *Func) SetResult(t Type, conv ResultConvention) *Func {
	f.typ.Results = []Result{{Type: t, Convention: conv}}
	return f
}

// SetThrows sets the error result type for throwing functions.
func (f *Func) SetThrows(t Type) *Func { f.typ.ErrorType = t; return f }

// Entry is the function's first block, created on first use.
func (f *Func) Entry() *Block {
	if len(f.blocks) == 0 {
		return f.Block()
	}
	return f.blocks[0]
}

// Block appends a new block.
func (f *Func) Block() *Block {
	b := &Block{fn: f, index: len(f.blocks)}
	f.blocks = append(f.blocks, b)
	return b
}

// RemoveBlock takes a block nothing branches to out of the function,
// numbering the blocks after it again.
func (f *Func) RemoveBlock(b *Block) {
	for i, x := range f.blocks {
		if x != b {
			continue
		}
		f.blocks = append(f.blocks[:i], f.blocks[i+1:]...)
		for j := i; j < len(f.blocks); j++ {
			f.blocks[j].index = j
		}
		return
	}
}

// Values returns all values defined within the function.
func (f *Func) Values() []*Value {
	var out []*Value
	for _, b := range f.blocks {
		out = append(out, b.args...)
		for _, in := range b.insts {
			out = append(out, in.results...)
		}
	}
	return out
}

// A Global is a module-level variable.
type Global struct {
	name    string
	linkage Linkage
	typ     Type
}

func (g *Global) Name() string     { return g.name }
func (g *Global) Linkage() Linkage { return g.linkage }
func (g *Global) Type() Type       { return g.typ }

// VTable represents a class virtual method dispatch table.
type VTable struct {
	Class   string
	Entries []TableEntry
	Layout  types.Type
	Deinit  string // Symbol name of class deinitializer, if any.
}

// WitnessTable represents a protocol conformance table.
type WitnessTable struct {
	Type           string
	Protocol       string
	Module         string
	Linkage        Linkage
	Entries        []TableEntry
	Symbol         string // Object symbol name for the witness table.
	Descriptor     string // Conformance descriptor symbol name.
	ProtocolSymbol string // Protocol descriptor symbol name.
}

// TableEntry represents an entry mapping a requirement to its implementation symbol.
type TableEntry struct {
	Member string // #Class.method or #Protocol.method
	Impl   string // the function's name
}
