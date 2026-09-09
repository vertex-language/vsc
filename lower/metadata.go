package lower

import (
	"github.com/vertex-language/ir"
	"github.com/vertex-language/vsc/types"
	"github.com/vertex-language/vsc/vil"
)

// Metadata: what a type declared here is, said in the form Swift's
// runtime reads.
//
// A value crossing into Swift carries its type with it, and the type
// is a pointer to a metadata record. Everything done with such a value
// without knowing what it is goes through that record: an existential
// is projected through it, copied through it and destroyed through it,
// and a generic function is handed it so that it can do the same.
//
// The shape is read out of a running program rather than remembered.
// A struct's metadata is
//
//	N-8   the value witness table
//	N+0   the kind, 0x200 for a struct
//	N+8   the nominal type descriptor
//	N+16  one four-byte field offset per stored property
//
// and the descriptor is seven four-byte fields, five of them relative
// pointers:
//
//	flags 0x51, parent, name, access function, fields, count, offset
//
// where the parent is the module's own descriptor and the offset is
// where the field offsets begin, in words -- two.
//
// # What is not here
//
// The reflection field descriptor, and the sections that let the
// runtime find any of this by name. Both are the same missing piece:
// a global cannot yet ask for a section of its own, so there is
// nowhere to put a `__swift5_types` entry. Nothing this compiler hands
// out needs one -- projecting, copying, destroying and calling a
// witness all start from the pointer rather than from a name -- but a
// `as?` against one of these types would not find it.
//
// # What is refused
//
// A type that owns something, and one too large for an existential's
// buffer. The first would need value witnesses that retain and
// release; the second would need them to allocate a box. Both are
// refused where they are used, in the same words they were refused in
// before there was any metadata at all.

// The layout of a value witness table, in words.
const (
	vwInitBufferWithCopyOfBuffer = 0
	vwDestroy                    = 1
	vwInitWithCopy               = 2
	vwAssignWithCopy             = 3
	vwInitWithTake               = 4
	vwAssignWithTake             = 5
	vwGetEnumTag                 = 6
	vwStoreEnumTag               = 7
	vwSize                       = 8
	vwStride                     = 9
	vwFlags                      = 10
	vwWords                      = 11
)

// metadataKind is what the record's first word says a type is.
const metadataKindStructRecord = 0x200

// The nominal type descriptor's flags: kind 17 is a struct, and 0x40
// says the descriptor is the unique one for that type. swiftc writes
// 0x51 for every struct declared without generic parameters.
const nominalStructFlags = 0x51

// fieldOffsetVectorWords is where a struct's field offsets begin,
// counted in words from the metadata pointer. Two: the kind and the
// descriptor come first.
const fieldOffsetVectorWords = 2

// allMetadata emits the metadata for every type this module said it
// would need, before any function body is lowered.
//
// The order is the reason: metadata is made of functions, and a
// function definition made while another one is being filled in lands
// in the middle of that one.
func (l *lowerer) allMetadata(m *vil.Module) error {
	for _, t := range m.MetadataTypes() {
		st, ok := structOf(vil.Object(t.Layout))
		if !ok || st == nil {
			return &Error{Err: ErrUnsupported, Func: t.Type,
				What: "a type whose layout this compiler does not have"}
		}
		if _, ok := l.metadataFor(t.Type, st); !ok {
			return &Error{Err: ErrUnsupported, Func: t.Type,
				What: "a type this compiler cannot describe at run time"}
		}
	}
	return nil
}

// metadataFor emits everything a type needs to be named at run time,
// once, and yields the address of its metadata.
//
// The metadata is not a symbol of its own. It sits eight bytes inside
// the record that begins with the value witness table pointer, and ir
// gives a global one name -- so what is returned is the record and the
// offset, and every reader adds them.
func (l *lowerer) metadataFor(typeName string, st *types.Struct) (*ir.Global, bool) {
	if l.meta == nil {
		l.meta = map[string]*ir.Global{}
	}
	if g, ok := l.meta[typeName]; ok {
		return g, g != nil
	}
	g, ok := l.buildMetadata(typeName, st)
	if !ok {
		l.meta[typeName] = nil
		return nil, false
	}
	l.meta[typeName] = g
	return g, true
}

// metadataOffset is how far into the record the metadata pointer
// points: past the value witness table pointer that precedes it.
const metadataOffset = 8

func (l *lowerer) buildMetadata(typeName string, st *types.Struct) (*ir.Global, bool) {
	info, ok := l.module.MetadataFor(typeName)
	if !ok || st == nil {
		return nil, false
	}
	size := types.Sizeof(st, types.DefaultTarget64)
	align := types.Alignof(st, types.DefaultTarget64)
	if size < 0 || align <= 0 || align > 8 {
		return nil, false
	}

	vwt := l.valueWitnessTable(info.Mangled+"WV", size, align)
	descr := l.nominalDescriptor(info, st)

	// The record: the value witness table, then the metadata proper.
	rec := l.out.Struct("meta_" + identSafe(typeName))
	rec.Field("vwt", ir.StorePtr.FType())
	rec.Field("kind", ir.StoreI64.FType())
	rec.Field("descriptor", ir.StorePtr.FType())
	vals := []ir.FieldVal{
		ir.Val("vwt", ir.RelocInit(vwt)),
		ir.Val("kind", ir.Lit(ir.Int(metadataKindStructRecord))),
		ir.Val("descriptor", ir.RelocInit(descr)),
	}
	for i, f := range st.Fields {
		if f == nil {
			return nil, false
		}
		off, ok := types.Offsetof(st, f.Name, types.DefaultTarget64)
		if !ok {
			return nil, false
		}
		name := "f" + itoa(i)
		rec.Field(name, ir.StoreI32.FType())
		vals = append(vals, ir.Val(name, ir.Lit(ir.Int(off))))
	}
	// Exported, as swiftc exports the same records: a type's
	// metadata and its descriptor are what another module names when
	// it mentions the type, and there is one of each per type.
	g := l.out.Global(l.sym(info.Mangled+"Mf"), ir.RO, rec.FType()).
		Init(ir.Fields(vals...)).
		Align(8)
	g.Export()
	l.metadataAccessorFor(info.Mangled+"Ma", g)
	return g, true
}

// valueWitnessTable is what is done to a value of the type without
// knowing what it is.
//
// Every one of them is a copy of the bytes or nothing at all, because
// the only types this emits metadata for are the ones that own
// nothing: swiftc's own table for such a struct points five of its
// six rows at one memcpy and the sixth at a function that returns.
//
// The two enum-tag rows are the standard library's own generic
// implementations, given a null callback -- which is what they take
// when the type has no spare representations to put a tag in, and
// this says it has none.
func (l *lowerer) valueWitnessTable(name string, size, align int64) *ir.Global {
	if g, ok := l.vwts[name]; ok {
		return g
	}
	copyFn := l.memcpyWitness(size)
	noop := l.noopWitness()
	get, store := l.enumTagWitnesses()

	rows := make([]ir.Init, vwWords)
	rows[vwInitBufferWithCopyOfBuffer] = ir.RelocInit(copyFn)
	rows[vwDestroy] = ir.RelocInit(noop)
	rows[vwInitWithCopy] = ir.RelocInit(copyFn)
	rows[vwAssignWithCopy] = ir.RelocInit(copyFn)
	rows[vwInitWithTake] = ir.RelocInit(copyFn)
	rows[vwAssignWithTake] = ir.RelocInit(copyFn)
	rows[vwGetEnumTag] = ir.RelocInit(get)
	rows[vwStoreEnumTag] = ir.RelocInit(store)
	rows[vwSize] = ir.Lit(ir.Int(size))
	// A type of no size still has a stride of one, so that an array
	// of them has distinct addresses. Swift says the same.
	stride := alignUp(size, align)
	if stride == 0 {
		stride = 1
	}
	rows[vwStride] = ir.Lit(ir.Int(stride))
	// The low byte is the alignment mask. Nothing else is set: the
	// value owns nothing, fits the buffer, and can be moved by copying
	// its bytes -- and the count of spare representations, which
	// shares this word, is zero.
	rows[vwFlags] = ir.Lit(ir.Int(align - 1))

	g := l.out.Global(l.sym(name), ir.RO,
		ir.Array(vwWords, ir.StorePtr.FType())).Init(ir.List(rows...)).Align(8)
	if l.vwts == nil {
		l.vwts = map[string]*ir.Global{}
	}
	l.vwts[name] = g
	return g
}

// memcpyWitness is the one function five of a trivial type's six
// witnesses are: the destination, the source, and the type, with the
// destination handed back.
func (l *lowerer) memcpyWitness(size int64) *ir.Func {
	name := l.sym("$sVSCcopy" + itoa(int(size)))
	if f, ok := l.witnessFns[name]; ok {
		return f
	}
	f := l.out.Func(name)
	f.Internal()
	dst := f.ParamPtr("dest")
	src := f.ParamPtr("src")
	f.ParamPtr("metadata")
	f.ReturnsPtr()
	b := f.Entry()
	b.MemCpy(dst, src, b.I64.Const(size))
	b.Return(dst)
	l.rememberWitness(name, f)
	return f
}

// noopWitness is destroy, for a value that owns nothing.
func (l *lowerer) noopWitness() *ir.Func {
	name := l.sym("$sVSCdestroy")
	if f, ok := l.witnessFns[name]; ok {
		return f
	}
	f := l.out.Func(name)
	f.Internal()
	f.ParamPtr("value")
	f.ParamPtr("metadata")
	f.Entry().Return()
	l.rememberWitness(name, f)
	return f
}

// enumTagWitnesses are the two rows that say where a tag goes when a
// value of this type is the payload of an enum the runtime lays out.
//
// Both trap. Which is not an implementation and is not meant to look
// like one: the layout depends on how many empty cases the enum has
// and on how many spare representations the payload has, and writing
// it out from memory rather than from a running program is how a
// wrong tag gets written silently. A trap says where it happened.
//
// Nothing this compiler produces reaches them. A value goes into an
// existential and comes out of one; it is copied and destroyed
// through the rows above; a generic function is handed the whole
// record and does the same. What would reach these two is a Swift
// library opening an existential this compiler filled in and forming
// an `Optional` of the type inside it -- which needs the type by
// name, and there is no section yet that would let it find one.
func (l *lowerer) enumTagWitnesses() (*ir.Func, *ir.Func) {
	get := l.sym("$sVSCgetEnumTag")
	store := l.sym("$sVSCstoreEnumTag")
	if f, ok := l.witnessFns[get]; ok {
		return f, l.witnessFns[store]
	}

	g := l.out.Func(get)
	g.Internal()
	g.ParamPtr("value")
	g.ParamI32("emptyCases")
	g.ParamPtr("metadata")
	g.ReturnsI32()
	g.Entry().Trap()

	s := l.out.Func(store)
	s.Internal()
	s.ParamPtr("value")
	s.ParamI32("whichCase")
	s.ParamI32("emptyCases")
	s.ParamPtr("metadata")
	s.Entry().Trap()

	l.rememberWitness(get, g)
	l.rememberWitness(store, s)
	return g, s
}

func (l *lowerer) rememberWitness(name string, f *ir.Func) {
	if l.witnessFns == nil {
		l.witnessFns = map[string]*ir.Func{}
	}
	l.witnessFns[name] = f
}

// nominalDescriptor is the record that says what a type is called and
// what module it is in.
//
// Five of its seven fields are relative pointers: a distance rather
// than an address, so that the table needs no relocation at load time.
// There is no symbol at an arbitrary offset inside it, so each field
// is written as its target minus the descriptor's own symbol, with the
// field's offset carried as a negative addend -- which is what
// swiftc's own descriptors do, entry by entry.
func (l *lowerer) nominalDescriptor(info vil.TypeMetadata, st *types.Struct) *ir.Global {
	name := l.sym(info.Mangled + "Mn")
	if g, ok := l.descriptors[name]; ok {
		return g
	}
	parent := l.moduleDescriptor(info)
	text := l.cstring(info.Mangled+"MnName", info.Name)

	g := l.out.Global(name, ir.RO, ir.Array(7, ir.StoreI32.FType()))
	g.Export()
	access := l.metadataAccessorName(info)
	g.Init(ir.List(
		ir.Lit(ir.Int(nominalStructFlags)),
		relative(parent, g, 1*4),
		relative(text, g, 2*4),
		relative(access, g, 3*4),
		// No reflection field descriptor: there is no section to put
		// one in yet, so nothing could find it. Null is what the
		// runtime reads as "this type does not describe its fields".
		ir.Lit(ir.Int(0)),
		ir.Lit(ir.Int(int64(len(st.Fields)))),
		ir.Lit(ir.Int(fieldOffsetVectorWords)),
	))
	if l.descriptors == nil {
		l.descriptors = map[string]*ir.Global{}
	}
	l.descriptors[name] = g
	return g
}

// moduleDescriptor names the module every type in it points back at.
func (l *lowerer) moduleDescriptor(info vil.TypeMetadata) *ir.Global {
	name := l.sym(info.ModuleSym)
	if g, ok := l.descriptors[name]; ok {
		return g
	}
	text := l.cstring(info.ModuleSym+"Name", info.Module)
	g := l.out.Global(name, ir.RO, ir.Array(3, ir.StoreI32.FType()))
	g.Export()
	g.Init(ir.List(
		// Kind zero is a module, and a module's descriptor is not
		// marked unique: there is one per module by construction.
		ir.Lit(ir.Int(0)),
		ir.Lit(ir.Int(0)),
		relative(text, g, 2*4),
	))
	if l.descriptors == nil {
		l.descriptors = map[string]*ir.Global{}
	}
	l.descriptors[name] = g
	return g
}

// relative is one field of a descriptor: the distance from the field
// to what it names.
func relative(to ir.Symbol, from ir.Symbol, off int64) ir.Init {
	return ir.RelocInit(to).Minus(from).Plus(ir.Int(-off))
}

// cstring places a NUL-terminated name.
func (l *lowerer) cstring(name, text string) *ir.Global {
	sym := l.sym(name)
	if g, ok := l.strings2[sym]; ok {
		return g
	}
	g := l.out.Global(sym, ir.RO,
		ir.Array(uint64(len(text)+1), ir.StoreI8.FType())).Init(ir.Str(text)).Align(1)
	if l.strings2 == nil {
		l.strings2 = map[string]*ir.Global{}
	}
	l.strings2[sym] = g
	return g
}

// metadataAccessorName is the accessor's symbol, declared before its
// body exists so that the descriptor can point at it.
func (l *lowerer) metadataAccessorName(info vil.TypeMetadata) *ir.Func {
	name := l.sym(info.Mangled + "Ma")
	if f, ok := l.witnessFns[name]; ok {
		return f
	}
	f := l.out.Func(name)
	f.Internal()
	l.rememberWitness(name, f)
	return f
}

// metadataAccessorFor fills in the accessor: it takes a request for
// how complete the metadata has to be and answers with the metadata
// and the state it is in. There is nothing to instantiate here, so the
// answer is the record's address and "complete", which is zero --
// which is what swiftc's own accessor for a non-generic struct is.
func (l *lowerer) metadataAccessorFor(name string, record *ir.Global) {
	f, ok := l.witnessFns[l.sym(name)]
	if !ok {
		return
	}
	f.ParamI64("request")
	f.Signature().Ret(ir.TypePtr).Ret(ir.TypeI64)
	b := f.Entry()
	b.Return(b.Ptr.Add(b.Ptr.GetAddr(record), b.I64.Const(metadataOffset)),
		b.I64.Const(0))
}

// The conformance descriptor's four fields, and the one bit of
// arithmetic in them.
//
// A relative pointer whose low bit is set does not name its target: it
// names a word that holds the target's address. That is how a
// descriptor reaches a protocol another image declares -- the distance
// to something in another dylib is not a link-time constant, so the
// distance is to a word here and the address goes in that word.
const relativeIndirect = 1

// conformanceDescriptor is the record that says what a witness table
// is a conformance of: which protocol, which type, and where the
// table is.
//
// Four four-byte fields, three of them relative pointers. The
// protocol's is indirect when the protocol was declared elsewhere,
// which is every protocol the standard library declares.
func (l *lowerer) conformanceDescriptor(t *vil.WitnessTable, table *ir.Global) (*ir.Global, bool) {
	if t.Descriptor == "" || t.Symbol == "" {
		return nil, false
	}
	info, ok := l.module.MetadataFor(t.Type)
	if !ok {
		return nil, false
	}
	typeDescr := l.descriptors[l.sym(info.Mangled+"Mn")]
	if typeDescr == nil {
		return nil, false
	}
	proto, indirect, ok := l.protocolDescriptor(t)
	if !ok {
		return nil, false
	}
	protoField := int64(0)
	if indirect {
		protoField = relativeIndirect
	}

	g := l.out.Global(l.sym(t.Descriptor), ir.RO, ir.Array(4, ir.StoreI32.FType()))
	g.Export()
	g.Init(ir.List(
		ir.RelocInit(proto).Minus(g).Plus(ir.Int(protoField)),
		relative(typeDescr, g, 1*4),
		relative(table, g, 2*4),
		// No flags: the conformance is unconditional, its type
		// reference is the descriptor itself, and it is not
		// retroactive.
		ir.Lit(ir.Int(0)),
	))
	return g, true
}

// protocolDescriptor is what a conformance points at, and whether the
// pointer has to go through a word of its own.
//
// A protocol this module declared would be a descriptor emitted here;
// this compiler emits none, so only an imported protocol answers --
// its descriptor is a symbol the declaring library exports, and a
// distance to another image is not a link-time constant. So the
// distance is to a word in this image, and the address goes in that
// word.
func (l *lowerer) protocolDescriptor(t *vil.WitnessTable) (ir.Symbol, bool, bool) {
	if t.ProtocolSymbol == "" {
		return nil, false, false
	}
	name := l.sym(t.ProtocolSymbol)
	slot := name + "_ref"
	if g, ok := l.descriptors[slot]; ok {
		return g, true, true
	}
	ext := l.out.ImportGlobal(name, ir.StorePtr.FType())
	g := l.out.Global(slot, ir.RO, ir.StorePtr.FType()).
		Init(ir.RelocInit(ext)).Align(8)
	g.Internal()
	if l.descriptors == nil {
		l.descriptors = map[string]*ir.Global{}
	}
	l.descriptors[slot] = g
	return g, true, true
}

// metadataRecord is a metadata record another library exports, which
// is referenced rather than called.
func (l *lowerer) metadataRecord(sym string) ir.Symbol {
	name := l.sym(sym)
	if g, ok := l.records[name]; ok {
		return g
	}
	g := l.out.ImportGlobal(name, ir.StorePtr.FType())
	if l.records == nil {
		l.records = map[string]*ir.GlobalImport{}
	}
	l.records[name] = g
	return g
}
