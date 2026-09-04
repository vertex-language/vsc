package vil

// A Linkage is how visible a function or a global is, spelled as SIL
// spells it. Public is the default and is written by saying nothing.
type Linkage string

const (
	Public         Linkage = ""
	Hidden         Linkage = "hidden"
	Private        Linkage = "private"
	Shared         Linkage = "shared"
	PublicExternal Linkage = "public_external"
	HiddenExternal Linkage = "hidden_external"
)

// A Func is one function: a name, a lowered type, and the blocks that
// implement it. A function with no blocks is a declaration — SIL
// writes it with no braces, and that is how an external symbol is
// referred to.
type Func struct {
	m       *Module
	name    string
	linkage Linkage
	typ     *FuncType
	attrs   []string
	blocks  []*Block
	nextID  int
}

func (f *Func) Module() *Module  { return f.m }
func (f *Func) Name() string     { return f.name }
func (f *Func) Linkage() Linkage { return f.linkage }
func (f *Func) Type() *FuncType  { return f.typ }
func (f *Func) Blocks() []*Block { return f.blocks }
func (f *Func) Attrs() []string  { return f.attrs }

// IsDeclaration reports whether the function has a body here.
func (f *Func) IsDeclaration() bool { return len(f.blocks) == 0 }

// SetLinkage sets how visible the function is.
func (f *Func) SetLinkage(l Linkage) *Func { f.linkage = l; return f }

// SetAttr adds a bracketed attribute — ossa, transparent, serialized,
// thunk — in the order they will be printed.
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

// OSSA reports whether the function is in ownership form. A raw
// module's functions are; a function loses the mark when ownership is
// lowered away on the road to VIR.
func (f *Func) OSSA() bool { return f.HasAttr("ossa") }

// Param adds a parameter, and its value in the entry block. Order is
// the order they are added, and the entry block's arguments are the
// parameters — which is why a parameter cannot be added once the
// entry block has instructions.
func (f *Func) Param(t Type, conv ParamConvention) *Value {
	f.typ.Params = append(f.typ.Params, Param{Type: t, Convention: conv})
	return f.Entry().Arg(t, ownershipOf(conv))
}

// ownershipOf is the ownership a parameter's value has inside the
// callee, which its convention decides.
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

// SetThrows gives the function an error result, which makes its calls
// try_apply rather than apply.
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

// Values walks every value the function defines, in block then
// instruction order.
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

// A VTable is a class's dynamic dispatch table: each method the class
// declares or overrides, and the function that implements it.
type VTable struct {
	Class   string
	Entries []TableEntry
}

// A WitnessTable is one conformance: the protocol, the type that
// conforms, and the function behind each requirement.
type WitnessTable struct {
	Type     string
	Protocol string
	Module   string
	Linkage  Linkage
	Entries  []TableEntry
}

// A TableEntry is one row of either table: the requirement, and what
// satisfies it.
type TableEntry struct {
	Member string // #Class.method or #Protocol.method
	Impl   string // the function's name
}
