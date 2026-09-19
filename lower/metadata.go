package lower

import (
	"github.com/vertex-language/ir"
	"github.com/vertex-language/vsc/internal/sil"
	"github.com/vertex-language/vsc/stdlib"
	"github.com/vertex-language/vsc/types"
	"strings"
)

// Metadata records and nominal type descriptors conforming to the Swift runtime ABI.
//
// Struct metadata layout:
//	N-8   Value witness table pointer
//	N+0   Kind (0x200 for struct)
//	N+8   Nominal type descriptor pointer
//	N+16  Field offset vector (int32 per stored property)

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

// metadataKindEnumRecord is an enum's: 0x201, or 0x202 were it Optional.
const metadataKindEnumRecord = 0x201

// nominalEnumFlags is an enum descriptor's: kind 18, unique.
const nominalEnumFlags = 0x52

// nominalStructFlags represents a non-generic nominal struct descriptor (kind 17 | unique 0x40).
const nominalStructFlags = 0x51

// fieldOffsetVectorWords is the offset in words from metadata pointer to field offsets.
const fieldOffsetVectorWords = 2

// allMetadata generates metadata records for all types required by the module before lowering functions.
func (l *lowerer) allMetadata(m *sil.Module) error {
	for _, t := range m.MetadataTypes() {
		if t.Imported {
			continue
		}
		switch t.Layout.Underlying().(type) {
		case *types.Class:
			if _, ok := l.classMetadataFor(t.Type); !ok {
				return &Error{Err: ErrUnsupported, Func: t.Name,
					What: "a class this compiler cannot describe at run time"}
			}
			continue
		case *types.Enum:
			if _, ok := l.enumMetadataFor(t.Type, t.Layout.Underlying().(*types.Enum)); !ok {
				return &Error{Err: ErrUnsupported, Func: t.Name,
					What: "an enum this compiler cannot describe at run time"}
			}
			continue
		case *types.Optional, *types.Array, *types.Dictionary, *types.Set, *types.Tuple:
			if _, ok := l.structuralFor(t.Layout); !ok {
				return &Error{Err: ErrUnsupported, Func: t.Name,
					What: "a type this compiler cannot describe at run time"}
			}
			continue
		}
		st, ok := structOf(sil.Object(t.Layout))
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

// metadataFor returns or generates the global metadata record for the named struct type.
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

// metadataOffset is the byte offset into the metadata record past the VWT pointer.
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

	owned, ok := ownedWords(st, 0)
	if !ok {
		return nil, false
	}
	var vwt *ir.Global
	if len(owned) == 0 {
		vwt = l.valueWitnessTable(info.Mangled+"WV", size, align)
	} else {
		vwt = l.ownedValueWitnessTable(info, size, align, owned)
	}
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

// enumMetadataFor returns or generates the metadata record for an enum.
// Only enums whose payloads own nothing are described: their values
// copy and destroy as the bytes they are.
func (l *lowerer) enumMetadataFor(typeName string, e *types.Enum) (*ir.Global, bool) {
	if l.meta == nil {
		l.meta = map[string]*ir.Global{}
	}
	if g, ok := l.meta[typeName]; ok {
		return g, g != nil
	}
	g, ok := l.buildEnumMetadata(typeName, e)
	l.meta[typeName] = g
	return g, ok
}

func (l *lowerer) buildEnumMetadata(typeName string, e *types.Enum) (*ir.Global, bool) {
	info, ok := l.module.MetadataFor(typeName)
	if !ok || e == nil {
		return nil, false
	}
	size := types.Sizeof(e, types.DefaultTarget64)
	align := types.Alignof(e, types.DefaultTarget64)
	if size < 0 || align <= 0 || align > 8 {
		return nil, false
	}
	var vwt *ir.Global
	if sil.Object(e).Trivial() {
		vwt = l.valueWitnessTable(info.Mangled+"WV", size, align)
	} else {
		// Its cases carry counted things: what a copy retains and a
		// destroy releases is what the active case carries.
		if vwt, ok = l.enumValueWitnessTable(info, e, size, align); !ok {
			return nil, false
		}
	}
	// The record is made before its descriptor, which a recursive
	// enum's payload names again: the descriptor finds it and stops.
	rec := l.out.Struct("meta_" + identSafe(typeName))
	rec.Field("vwt", ir.StorePtr.FType())
	rec.Field("kind", ir.StoreI64.FType())
	rec.Field("descriptor", ir.StorePtr.FType())
	g := l.out.Global(l.sym(info.Mangled+"Mf"), ir.RO, rec.FType()).Align(8)
	l.meta[typeName] = g
	// No fields to describe: an empty struct stands in, and the
	// descriptor says so by its count.
	descr := l.enumDescriptor(info, e)
	g.Init(ir.Fields(
		ir.Val("vwt", ir.RelocInit(vwt)),
		ir.Val("kind", ir.Lit(ir.Int(metadataKindEnumRecord))),
		ir.Val("descriptor", ir.RelocInit(descr)),
	))
	g.Export()
	l.metadataAccessorFor(info.Mangled+"Ma", g)
	return g, true
}

// valueWitnessTable generates a value witness table for a trivial (POD) type.
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
	rows[vwFlags] = ir.Lit(ir.Int(witnessFlags(size, align, false)))

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

// enumTagWitnesses returns fallback trap witnesses for getEnumTag and storeEnumTag.
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

// nominalDescriptor generates a nominal type descriptor for a struct.
func (l *lowerer) nominalDescriptor(info sil.TypeMetadata, st *types.Struct) *ir.Global {
	return l.nominalDescriptorWith(info, st, nominalStructFlags)
}

// nominalClassFlags is a class descriptor's: kind 16, unique.
const nominalClassFlags = 0x50

func (l *lowerer) nominalDescriptorWith(info sil.TypeMetadata, st *types.Struct, flags int64) *ir.Global {
	name := l.sym(info.Mangled + "Mn")
	if g, ok := l.descriptors[name]; ok {
		return g
	}
	parent := l.moduleDescriptor(info)
	text := l.cstring(info.Mangled+"MnName", info.Name)

	g := l.out.Global(name, ir.RO, ir.Array(7, ir.StoreI32.FType()))
	g.Export()
	fields := ir.Init(ir.Lit(ir.Int(0)))
	if fd := l.fieldDescriptor(info, st); fd != nil {
		fields = relative(fd, g, 4*4)
	}
	access := l.metadataAccessorName(info)
	g.Init(ir.List(
		ir.Lit(ir.Int(flags)),
		relative(parent, g, 1*4),
		relative(text, g, 2*4),
		relative(access, g, 3*4),
		// The fields, by name and by the metadata of their types: what
		// print needs to write `Point(x: 1, y: 2)`. Null where a field's
		// type has no metadata to point at, which the runtime reads as
		// "this type does not describe its fields".
		fields,
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
func (l *lowerer) moduleDescriptor(info sil.TypeMetadata) *ir.Global {
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
func (l *lowerer) metadataAccessorName(info sil.TypeMetadata) *ir.Func {
	name := l.sym(info.Mangled + "Ma")
	if f, ok := l.witnessFns[name]; ok {
		return f
	}
	f := l.out.Func(name)
	// A public type's accessor is called by name from another module, so
	// it stays a symbol this one exports. Everything else is this
	// module's own business.
	if !info.Public {
		f.Internal()
	}
	l.rememberWitness(name, f)
	return f
}

// metadataAccessorFor generates a metadata accessor function returning the metadata pointer and completion state.
func (l *lowerer) metadataAccessorFor(name string, record *ir.Global) {
	f, ok := l.witnessFns[l.sym(name)]
	if !ok {
		f = l.out.Func(l.sym(name))
		f.Export()
		l.rememberWitness(l.sym(name), f)
	}
	f.ParamI64("request")
	b := f.Entry()
	meta := b.Ptr.Add(b.Ptr.GetAddr(record), b.I64.Const(metadataOffset))
	// The state word is left off where the convention returns one
	// register. Nothing this compiler emits reads it; a Swift library
	// calling the accessor would, and on Windows there is none.
	if l.ms {
		f.Signature().Ret(ir.TypePtr)
		b.Return(meta)
		return
	}
	f.Signature().Ret(ir.TypePtr).Ret(ir.TypeI64)
	b.Return(meta, b.I64.Const(0))
}

// relativeIndirect flags a relative pointer as indirect (pointing to a GOT-like pointer slot).
const relativeIndirect = 1

// conformanceDescriptor generates a protocol conformance descriptor linking type, protocol, and witness table.
func (l *lowerer) conformanceDescriptor(t *sil.WitnessTable, table *ir.Global) (*ir.Global, bool) {
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
	l.conformanceRecord(t.Descriptor, g)
	return g, true
}

// ConformanceSection is where every conformance a module declares is
// listed, one relative pointer to its descriptor per record, for the
// runtime to find a type's conformances by: Swift's __swift5_proto, under
// a name of Vertex's own so that a Swift runtime linked beside it does not
// read records it did not write.
const ConformanceSection = "__TEXT,__vertex_proto,regular,no_dead_strip"

// conformanceRecord lists a conformance descriptor where the runtime
// looks for them.
func (l *lowerer) conformanceRecord(descriptor string, d *ir.Global) {
	g := l.out.Global(l.sym(descriptor+"_record"), ir.RO, ir.StoreI32.FType())
	g.Internal()
	g.Section(ConformanceSection)
	g.Init(ir.RelocInit(d).Minus(g))
}

// protocolDescriptors defines the descriptor of each protocol the module
// declares: Swift's layout -- flags, parent, name, the counts, associated
// type names -- with the kind (3, a protocol) in the flags and the rest
// empty. What a conformance needs of it is that it is one address.
func (l *lowerer) protocolDescriptors(m *sil.Module) {
	for _, sym := range m.ProtocolDescriptors() {
		name := l.sym(sym)
		if _, ok := l.descriptors[name]; ok {
			continue
		}
		g := l.out.Global(name, ir.RO, ir.Array(6, ir.StoreI32.FType()))
		g.Export()
		g.Init(ir.List(ir.Lit(ir.Int(3)), ir.Lit(ir.Int(0)), ir.Lit(ir.Int(0)),
			ir.Lit(ir.Int(0)), ir.Lit(ir.Int(0)), ir.Lit(ir.Int(0))))
		if l.descriptors == nil {
			l.descriptors = map[string]*ir.Global{}
		}
		l.descriptors[name] = g
	}
}

// protocolDescriptor returns the symbol and indirect flag for a protocol descriptor referenced by a conformance.
func (l *lowerer) protocolDescriptor(t *sil.WitnessTable) (ir.Symbol, bool, bool) {
	if t.ProtocolSymbol == "" {
		return nil, false, false
	}
	name := l.sym(t.ProtocolSymbol)
	// Defined in this module: pointed at directly.
	if g, ok := l.descriptors[name]; ok {
		return g, false, true
	}
	slot := name + "_ref"
	if g, ok := l.descriptors[slot]; ok {
		return g, true, true
	}
	ext := l.out.ImportGlobal(name, ir.StorePtr.FType())
	// The standard library's are the runtime's, or Swift's: weak, so an
	// object linked with neither -- a library driven from C -- still links,
	// its conformances unfound rather than unresolved.
	if strings.HasPrefix(t.ProtocolSymbol, "$ss") || strings.HasPrefix(t.ProtocolSymbol, "$sS") {
		ext.Weak()
	}
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

// fieldDescriptor generates field reflection records (count, name pointer, metadata pointer per field).
func (l *lowerer) fieldDescriptor(info sil.TypeMetadata, st *types.Struct) *ir.Global {
	rec := l.out.Struct("fields_" + identSafe(info.Type))
	rec.Field("count", ir.StoreI64.FType())
	vals := []ir.FieldVal{ir.Val("count", ir.Lit(ir.Int(int64(len(st.Fields)))))}
	for i, f := range st.Fields {
		if f == nil {
			return nil
		}
		record, ok := l.fieldTypeRecord(f.Type)
		if !ok {
			return nil
		}
		name := l.cstring(info.Mangled+"MF"+itoa(i), f.Name)
		rec.Field("name"+itoa(i), ir.StorePtr.FType())
		rec.Field("type"+itoa(i), ir.StorePtr.FType())
		vals = append(vals, ir.Val("name"+itoa(i), ir.RelocInit(name)),
			ir.Val("type"+itoa(i), ir.RelocInit(record)))
	}
	return l.out.Global(l.sym(info.Mangled+"MF"), ir.RO, rec.FType()).
		Init(ir.Fields(vals...)).Align(8)
}

// fieldTypeRecord is the metadata record for a field's type: the
// runtime's for a type the core declares, this module's for a struct
// declared here or an Optional or Array of something.
func (l *lowerer) fieldTypeRecord(t types.Type) (ir.Symbol, bool) {
	switch t.Underlying().(type) {
	case *types.Optional, *types.Array, *types.Dictionary, *types.Set, *types.Tuple:
		if _, known := l.module.MetadataFor(sil.StructuralKey(t)); !known {
			return nil, false
		}
		g, ok := l.structuralFor(t)
		return g, ok
	}
	if t == nil {
		return nil, false
	}
	switch ex := t.(type) {
	case *types.Existential:
		switch len(ex.Protocols) {
		case 0:
			return l.metadataRecord(stdlib.Metadata("Any")), true
		case 1:
			return l.metadataRecord(stdlib.Metadata("Existential1")), true
		}
		return nil, false
	case *types.Protocol:
		return l.metadataRecord(stdlib.Metadata("Existential1")), true
	}
	if b, ok := t.Underlying().(*types.Basic); ok {
		name, known := bridgeableElements[b.Kind()]
		if b.Kind() == types.String {
			name, known = "String", true
		}
		if !known {
			return nil, false
		}
		return l.metadataRecord(stdlib.Metadata(name)), true
	}
	// Another module's type: the record that module exports.
	if info, known := l.module.MetadataFor(typeNameOfType(sil.Object(t))); known && info.Imported {
		return l.metadataRecord(info.Mangled + "Mf"), true
	}
	if _, isClass := t.Underlying().(*types.Class); isClass {
		g, ok := l.classMetadataFor(typeNameOfType(sil.Object(t)))
		return g, ok
	}
	if e, isEnum := t.Underlying().(*types.Enum); isEnum {
		name := typeNameOfType(sil.Object(t))
		if _, known := l.module.MetadataFor(name); !known {
			return nil, false
		}
		return l.enumMetadataFor(name, e)
	}
	st, ok := t.Underlying().(*types.Struct)
	if !ok {
		return nil, false
	}
	name := typeNameOfType(sil.Object(t))
	if _, known := l.module.MetadataFor(name); !known {
		return nil, false
	}
	g, ok := l.metadataFor(name, st)
	if !ok {
		return nil, false
	}
	return g, true
}

// An ownedWord is one word inside a value that is a reference: the
// object word of a String, or an Array's or a class instance's pointer.
type ownedWord struct {
	offset int64
	string bool
	// enum is a payload enum starting at offset, whose owned words are
	// those its active case carries, found by its tag.
	enum *types.Enum
}

// ownedWords returns the byte offsets of reference-counted words within a struct.
func ownedWords(st *types.Struct, base int64) ([]ownedWord, bool) {
	var out []ownedWord
	for _, f := range st.Fields {
		if f == nil || f.Type == nil {
			return nil, false
		}
		off, ok := types.Offsetof(st, f.Name, types.DefaultTarget64)
		if !ok {
			return nil, false
		}
		at := base + off
		switch u := f.Type.Underlying().(type) {
		case *types.Basic:
			if u.Kind() == types.String {
				out = append(out, ownedWord{offset: at + stdlib.StringObjectWord*8, string: true})
				continue
			}
			if !sil.Object(f.Type).Trivial() {
				return nil, false
			}
		case *types.Array, *types.Dictionary, *types.Set, *types.Class:
			out = append(out, ownedWord{offset: at})
		case *types.Signature:
			// A function value's context, its second word.
			out = append(out, ownedWord{offset: at + 8})
		case *sil.BoxType:
			out = append(out, ownedWord{offset: at})
		case *types.Struct:
			inner, ok := ownedWords(u, at)
			if !ok {
				return nil, false
			}
			out = append(out, inner...)
		case *types.Tuple:
			image, ok := tupleImage(u)
			if !ok {
				return nil, false
			}
			inner, ok := ownedWords(image, at)
			if !ok {
				return nil, false
			}
			out = append(out, inner...)
		case *types.Optional:
			// What an optional owns is what it wraps, found where the
			// wrapped value keeps it. None is words of zero, which a
			// retain and a release both take as nothing.
			if st, _, spare := spareStructOptional(u); spare {
				inner, ok := ownedWords(st, at)
				if !ok {
					return nil, false
				}
				out = append(out, inner...)
				continue
			}
			// One with a tag byte wrapping a struct or a tuple holds
			// what the payload holds, before the tag, and zeros where
			// it is none.
			if st, ok := taggedAggregateOptional(u); ok {
				inner, ok := ownedWords(st, at)
				if !ok {
					return nil, false
				}
				out = append(out, inner...)
				continue
			}
			// One with a tag byte wrapping a payload enum holds what the
			// enum's own tag says it does, and zeros where it is none.
			if e, ok := taggedEnumOptional(u); ok {
				out = append(out, ownedWord{offset: at, enum: e})
				continue
			}
			word, owns, ok := optionalOwned(u)
			if !ok {
				return nil, false
			}
			if owns {
				word.offset += at
				out = append(out, word)
			}
		default:
			if sil.Object(f.Type).Trivial() {
				continue
			}
			// A payload enum whose cases carry counted things holds what
			// its tag says.
			if e, ok := f.Type.Underlying().(*types.Enum); ok && hasPayload(e) && enumCountable(e) {
				out = append(out, ownedWord{offset: at, enum: e})
				continue
			}
			return nil, false
		}
	}
	return out, true
}

// countOwned retains or releases what the value at value holds -- each
// owned word, and each counted enum by what its active case carries --
// starting in b, and is the block that follows.
func countOwned(f *ir.Func, b *ir.Block, value ir.Ptr, owned []ownedWord,
	strings, objects ir.Callee, label string) *ir.Block {
	for i, w := range owned {
		if w.enum == nil {
			word := b.Ptr.Load(b.Ptr.Add(value, b.I64.Const(w.offset)))
			if w.string {
				b.Call(strings, word)
			} else {
				b.Call(objects, word)
			}
			continue
		}
		e := w.enum
		prefix := label + "e" + itoa(i) + "_"
		done := f.Block(prefix + "done")
		tag := b.I32.Const(0)
		if len(e.Cases) > 1 {
			tag = b.I32.ULoad8(b.Ptr.Add(value, b.I64.Const(w.offset+payloadArea(e))))
		}
		for k, c := range e.Cases {
			if c == nil || c.AssociatedType == nil || sil.Object(sil.CaseStorage(c)).Trivial() {
				continue
			}
			t, _ := enumTagOf(e, c.Name)
			inner, _ := caseOwned(sil.CaseStorage(c))
			shifted := make([]ownedWord, len(inner))
			for j, x := range inner {
				x.offset += w.offset
				shifted[j] = x
			}
			body := f.Block(prefix + "case" + itoa(k))
			next := f.Block(prefix + "next" + itoa(k))
			b.BrIf(b.I32.Eq(tag, b.I32.Const(t)), body.To(), next.To())
			countOwned(f, body, value, shifted, strings, objects, prefix+"c"+itoa(k)+"_").Br(done.To())
			b = next
		}
		b.Br(done.To())
		b = done
	}
	return b
}

// ownedValueWitnessTable generates a value witness table for a struct containing reference fields.
func (l *lowerer) ownedValueWitnessTable(info sil.TypeMetadata, size, align int64, owned []ownedWord) *ir.Global {
	name := info.Mangled + "WV"
	if g, ok := l.vwts[name]; ok {
		return g
	}
	retain := l.runtimeFunc(stdlib.Retain, ir.NewSig().Param(ir.TypePtr))
	release := l.runtimeFunc(stdlib.Release, ir.NewSig().Param(ir.TypePtr))
	retainString := l.runtimeFunc(stdlib.StringRetain, ir.NewSig().Param(ir.TypePtr))
	releaseString := l.runtimeFunc(stdlib.StringRelease, ir.NewSig().Param(ir.TypePtr))
	each := func(f *ir.Func, b *ir.Block, value ir.Ptr, strings, objects ir.Callee, label string) *ir.Block {
		return countOwned(f, b, value, owned, strings, objects, label)
	}
	fn := func(suffix string) *ir.Func {
		f := l.out.Func(l.sym(info.Mangled + suffix))
		f.Internal()
		return f
	}

	// initializeWithCopy: the bytes, then a retain for each reference.
	copyFn := fn("WVcopy")
	{
		dst, src := copyFn.ParamPtr("dest"), copyFn.ParamPtr("src")
		copyFn.ParamPtr("metadata")
		copyFn.ReturnsPtr()
		b := copyFn.Entry()
		b.MemCpy(dst, src, b.I64.Const(size))
		each(copyFn, b, dst, retainString, retain, "r").Return(dst)
	}
	// destroy: a release for each reference.
	destroyFn := fn("WVdestroy")
	{
		v := destroyFn.ParamPtr("value")
		destroyFn.ParamPtr("metadata")
		b := destroyFn.Entry()
		each(destroyFn, b, v, releaseString, release, "d").Return()
	}
	// assignWithCopy: retain what arrives before releasing what leaves,
	// so that assigning a value to itself keeps it alive.
	assignCopyFn := fn("WVassigncopy")
	{
		dst, src := assignCopyFn.ParamPtr("dest"), assignCopyFn.ParamPtr("src")
		assignCopyFn.ParamPtr("metadata")
		assignCopyFn.ReturnsPtr()
		b := assignCopyFn.Entry()
		b = each(assignCopyFn, b, src, retainString, retain, "s")
		b = each(assignCopyFn, b, dst, releaseString, release, "t")
		b.MemCpy(dst, src, b.I64.Const(size))
		b.Return(dst)
	}
	// initializeWithTake: the bytes, whose references move with them.
	takeFn := l.memcpyWitness(size)
	// assignWithTake: release what leaves, then the bytes.
	assignTakeFn := fn("WVassigntake")
	{
		dst, src := assignTakeFn.ParamPtr("dest"), assignTakeFn.ParamPtr("src")
		assignTakeFn.ParamPtr("metadata")
		assignTakeFn.ReturnsPtr()
		b := assignTakeFn.Entry()
		b = each(assignTakeFn, b, dst, releaseString, release, "d")
		b.MemCpy(dst, src, b.I64.Const(size))
		b.Return(dst)
	}
	get, store := l.enumTagWitnesses()

	rows := make([]ir.Init, vwWords)
	rows[vwInitBufferWithCopyOfBuffer] = ir.RelocInit(copyFn)
	rows[vwDestroy] = ir.RelocInit(destroyFn)
	rows[vwInitWithCopy] = ir.RelocInit(copyFn)
	rows[vwAssignWithCopy] = ir.RelocInit(assignCopyFn)
	rows[vwInitWithTake] = ir.RelocInit(takeFn)
	rows[vwAssignWithTake] = ir.RelocInit(assignTakeFn)
	rows[vwGetEnumTag] = ir.RelocInit(get)
	rows[vwStoreEnumTag] = ir.RelocInit(store)
	rows[vwSize] = ir.Lit(ir.Int(size))
	stride := alignUp(size, align)
	if stride == 0 {
		stride = 1
	}
	rows[vwStride] = ir.Lit(ir.Int(stride))
	rows[vwFlags] = ir.Lit(ir.Int(witnessFlags(size, align, true)))

	g := l.out.Global(l.sym(name), ir.RO,
		ir.Array(vwWords, ir.StorePtr.FType())).Init(ir.List(rows...)).Align(8)
	if l.vwts == nil {
		l.vwts = map[string]*ir.Global{}
	}
	l.vwts[name] = g
	return g
}

// structRefCount retains or releases every reference a struct holds.
func (c *fn) structRefCount(in *sil.Inst, st *types.Struct, retain bool) error {
	owned, ok := ownedWords(st, 0)
	if !ok {
		return c.fail(ErrUnsupported, in.Op(), "a struct holding something whose copy is not a retain")
	}
	if len(owned) == 0 {
		return nil
	}
	// The references are read where the struct is: out of its storage
	// when it lives in memory, or out of the flat list of registers it
	// was taken apart into, where a String is its two words and an enum
	// its words.
	widthOf := func(w ownedWord) int {
		if w.enum == nil {
			return 1
		}
		image, _ := enumImage(w.enum)
		return len(image.Fields)
	}
	words := make([][]ir.Value, len(owned))
	if from, inMemory := c.mem[in.Args()[0]]; inMemory {
		for i, w := range owned {
			for k := 0; k < widthOf(w); k++ {
				words[i] = append(words[i], c.b.I64.Load(c.fieldAddr(from, w.offset+int64(k*8))))
			}
		}
	} else {
		parts, held := c.parts(in.Args()[0])
		if !held {
			// A struct of one scalar, a Task's handle, is that one register.
			one, isOne := c.value(in.Args()[0])
			if !isOne {
				return c.fail(ErrUnsupported, in.Op(), "a struct that is neither in registers nor in memory")
			}
			parts = []ir.Value{one}
		}
		indices, ok := ownedLeaves(st, 0)
		if !ok || len(indices) != len(owned) {
			return c.fail(ErrUnsupported, in.Op(), "a struct whose references cannot be found among its registers")
		}
		for i, at := range indices {
			n := widthOf(owned[i])
			if at+n > len(parts) {
				return c.fail(ErrUnsupported, in.Op(), "a reference past the struct's registers")
			}
			words[i] = parts[at : at+n]
		}
	}
	objects, strings := stdlib.Release, stdlib.StringRelease
	if retain {
		objects, strings = stdlib.Retain, stdlib.StringRetain
	}
	for i, w := range owned {
		if w.enum != nil {
			if err := c.countEnumWords(in, w.enum, words[i], retain); err != nil {
				return err
			}
			continue
		}
		p, err := c.asPointer(in, words[i][0])
		if err != nil {
			return err
		}
		name := objects
		if w.string {
			name = strings
		}
		c.b.Call(c.l.runtimeFunc(name, ir.NewSig().Param(ir.TypePtr)), p)
	}
	return nil
}

// ownedLeaves is where each of ownedWords' references is in the flat
// list of registers a struct is held in, in the same order: a String is
// two of them and its reference the second, a nested struct its own
// list.
func ownedLeaves(st *types.Struct, base int) ([]int, bool) {
	var out []int
	at := base
	for _, f := range st.Fields {
		if f == nil || f.Type == nil {
			return nil, false
		}
		switch u := f.Type.Underlying().(type) {
		case *types.Basic:
			if u.Kind() == types.String {
				out = append(out, at+stdlib.StringObjectWord)
				at += 2
				continue
			}
			at++
		case *types.Signature:
			// A function value's context, its second register.
			out = append(out, at+1)
			at += 2
		case *types.Array, *types.Dictionary, *types.Set, *types.Class:
			out = append(out, at)
			at++
		case *types.Enum:
			if image, ok := enumImage(u); ok {
				if !sil.Object(f.Type).Trivial() && enumCountable(u) {
					out = append(out, at)
				}
				at += len(image.Fields)
				continue
			}
			at++
		case *types.Struct, *types.Tuple:
			st, isStruct := u.(*types.Struct)
			if !isStruct {
				image, ok := tupleImage(u.(*types.Tuple))
				if !ok {
					return nil, false
				}
				st = image
			}
			inner, ok := ownedLeaves(st, at)
			if !ok {
				return nil, false
			}
			out = append(out, inner...)
			n, ok := leafCount(st)
			if !ok {
				return nil, false
			}
			at += n
		case *types.Optional:
			// A struct that spares a representation: its own registers.
			if inner, _, spare := spareStructOptional(u); spare {
				owned, ok := ownedLeaves(inner, at)
				if !ok {
					return nil, false
				}
				out = append(out, owned...)
				n, ok := leafCount(inner)
				if !ok {
					return nil, false
				}
				at += n
				continue
			}
			// One with a tag byte wrapping a struct or a tuple: the
			// payload's registers, then the tag's.
			if inner, ok := taggedAggregateOptional(u); ok {
				owned, ok := ownedLeaves(inner, at)
				if !ok {
					return nil, false
				}
				out = append(out, owned...)
				n, ok := optionalLeafCount(u)
				if !ok {
					return nil, false
				}
				at += n
				continue
			}
			if _, ok := taggedEnumOptional(u); ok {
				out = append(out, at)
				n, ok := optionalLeafCount(u)
				if !ok {
					return nil, false
				}
				at += n
				continue
			}
			// Its registers, as appendLeaves lays it out, and the one
			// holding what it owns, if it owns anything.
			word, owns, ok := optionalOwned(u)
			if !ok {
				return nil, false
			}
			if owns {
				out = append(out, at+int(word.offset/8))
			}
			n, ok := optionalLeafCount(u)
			if !ok {
				return nil, false
			}
			at += n
		default:
			at++
		}
	}
	return out, true
}

// leafCount is how many registers a struct is held in, flat.
func leafCount(st *types.Struct) (int, bool) {
	n := 0
	for _, f := range st.Fields {
		if f == nil || f.Type == nil {
			return 0, false
		}
		switch u := f.Type.Underlying().(type) {
		case *types.Signature:
			// Its code and its context. See funcWords.
			n += 2
			continue
		case *types.Basic:
			if u.Kind() == types.String {
				n += 2
				continue
			}
			n++
		case *types.Enum:
			if image, ok := enumImage(u); ok {
				n += len(image.Fields)
				continue
			}
			n++
		case *types.Struct:
			k, ok := leafCount(u)
			if !ok {
				return 0, false
			}
			n += k
		case *types.Tuple:
			image, ok := tupleImage(u)
			if !ok {
				return 0, false
			}
			k, ok := leafCount(image)
			if !ok {
				return 0, false
			}
			n += k
		case *types.Optional:
			k, ok := optionalLeafCount(u)
			if !ok {
				return 0, false
			}
			n += k
		default:
			n++
		}
	}
	return n, true
}

// optionalOwned is the word an optional field owns, at an offset within
// the optional, and whether it owns one: a String's object word, a
// function's context, or the reference itself for an optional reference.
// It is not ok for an optional of something else that owns references.
func optionalOwned(o *types.Optional) (ownedWord, bool, bool) {
	switch {
	case isStringOptional(o):
		return ownedWord{offset: stdlib.StringObjectWord * 8, string: true}, true, true
	case isFuncOptional(o):
		return ownedWord{offset: 8}, true, true
	case sil.Object(o).Trivial():
		return ownedWord{}, false, true
	}
	if _, tagged := optionalImage(o); tagged {
		return ownedWord{}, false, false
	}
	switch o.Wrapped.Underlying().(type) {
	case *types.Array, *types.Dictionary, *types.Set, *types.Class:
		return ownedWord{}, true, true
	}
	return ownedWord{}, false, false
}

// taggedAggregateOptional is the struct an optional with a tag byte wraps
// -- a tuple as the struct its image is -- whose references sit where the
// payload keeps them, and are zero words where the optional is none.
func taggedAggregateOptional(o *types.Optional) (*types.Struct, bool) {
	if _, tagged := optionalImage(o); !tagged {
		return nil, false
	}
	switch u := o.Wrapped.Underlying().(type) {
	case *types.Struct:
		if len(u.TypeParams) > 0 {
			return nil, false
		}
		return u, true
	case *types.Tuple:
		return tupleImage(u)
	}
	return nil, false
}

// taggedEnumOptional is the payload enum an optional with a tag byte
// wraps, where the enum's cases carry counted things: what it owns is
// what the enum's own tag says, read from the enum's words.
func taggedEnumOptional(o *types.Optional) (*types.Enum, bool) {
	if _, tagged := optionalImage(o); !tagged {
		return nil, false
	}
	e, ok := o.Wrapped.Underlying().(*types.Enum)
	if !ok || !hasPayload(e) || !enumCountable(e) || sil.Object(o.Wrapped).Trivial() {
		return nil, false
	}
	return e, true
}

// optionalLeafCount is how many registers an optional field is held in:
// a String's or a function's two, the payload's and its tag for one that
// needs a tag byte, and one for a reference that spares null.
func optionalLeafCount(o *types.Optional) (int, bool) {
	if isStringOptional(o) || isFuncOptional(o) {
		return 2, true
	}
	if st, _, ok := spareStructOptional(o); ok {
		ls, ok := structLeaves(st)
		if !ok {
			return 0, false
		}
		return len(ls), true
	}
	if image, tagged := optionalImage(o); tagged {
		ls, ok := structLeaves(image)
		if !ok {
			return 0, false
		}
		return len(ls), true
	}
	return 1, true
}

// structuralFor returns or generates metadata for structural types (Optional, Array, Dictionary, Set).
func (l *lowerer) structuralFor(t types.Type) (*ir.Global, bool) {
	key := sil.StructuralKey(t)
	if l.meta == nil {
		l.meta = map[string]*ir.Global{}
	}
	if g, ok := l.meta[key]; ok {
		return g, g != nil
	}
	l.meta[key] = nil
	info, ok := l.module.MetadataFor(key)
	if !ok {
		return nil, false
	}
	size := types.Sizeof(t, types.DefaultTarget64)
	align := types.Alignof(t, types.DefaultTarget64)
	if size <= 0 || align <= 0 || align > 8 {
		return nil, false
	}
	fn := func(name string) ir.Init { return ir.RelocInit(l.runtimeWitness(name)) }

	rec := l.out.Struct("meta_" + identSafe(info.Mangled))
	rec.Field("vwt", ir.StorePtr.FType())
	rec.Field("kind", ir.StoreI64.FType())
	rows := make([]ir.Init, vwWords)
	var vals []ir.FieldVal
	flags := align - 1

	switch u := t.Underlying().(type) {
	case *types.Optional:
		payload, ok := l.fieldTypeRecord(u.Wrapped)
		if !ok {
			return nil, false
		}
		tagOffset, tagBytes, none, ok := optionalEmptyCase(u)
		if !ok {
			return nil, false
		}
		rows[vwInitBufferWithCopyOfBuffer] = fn(stdlib.WitnessOptionalCopy)
		rows[vwDestroy] = fn(stdlib.WitnessOptionalDestroy)
		rows[vwInitWithCopy] = fn(stdlib.WitnessOptionalCopy)
		rows[vwAssignWithCopy] = fn(stdlib.WitnessOptionalAssignCopy)
		rows[vwAssignWithTake] = fn(stdlib.WitnessOptionalAssignTake)
		if !sil.Object(u.Wrapped).Trivial() {
			flags |= stdlib.WitnessIsNonPOD
		}
		rec.Field("payload", ir.StorePtr.FType())
		rec.Field("tagOffset", ir.StoreI32.FType())
		rec.Field("tagBytes", ir.StoreI32.FType())
		rec.Field("none", ir.StoreI64.FType())
		vals = []ir.FieldVal{
			ir.Val("kind", ir.Lit(ir.Int(stdlib.KindOptional))),
			ir.Val("payload", ir.RelocInit(payload).Plus(ir.Int(stdlib.MetadataOffset))),
			ir.Val("tagOffset", ir.Lit(ir.Int(tagOffset))),
			ir.Val("tagBytes", ir.Lit(ir.Int(tagBytes))),
			ir.Val("none", ir.Lit(ir.Int(none))),
		}
	case *types.Array:
		element, ok := l.fieldTypeRecord(u.Elem)
		if !ok {
			return nil, false
		}
		rows[vwInitBufferWithCopyOfBuffer] = fn(stdlib.WitnessReferenceCopy)
		rows[vwDestroy] = fn(stdlib.WitnessReferenceDestroy)
		rows[vwInitWithCopy] = fn(stdlib.WitnessReferenceCopy)
		rows[vwAssignWithCopy] = fn(stdlib.WitnessReferenceAssignCopy)
		rows[vwAssignWithTake] = fn(stdlib.WitnessReferenceAssignTake)
		flags |= stdlib.WitnessIsNonPOD
		rec.Field("element", ir.StorePtr.FType())
		vals = []ir.FieldVal{
			ir.Val("kind", ir.Lit(ir.Int(stdlib.KindArray))),
			ir.Val("element", ir.RelocInit(element).Plus(ir.Int(stdlib.MetadataOffset))),
		}
	case *types.Dictionary, *types.Set:
		// One reference to a hash table, whose key and value types the
		// record says for describing it.
		rows[vwInitBufferWithCopyOfBuffer] = fn(stdlib.WitnessReferenceCopy)
		rows[vwDestroy] = fn(stdlib.WitnessReferenceDestroy)
		rows[vwInitWithCopy] = fn(stdlib.WitnessReferenceCopy)
		rows[vwAssignWithCopy] = fn(stdlib.WitnessReferenceAssignCopy)
		rows[vwAssignWithTake] = fn(stdlib.WitnessReferenceAssignTake)
		flags |= stdlib.WitnessIsNonPOD
		if d, isDict := u.(*types.Dictionary); isDict {
			key, ok := l.fieldTypeRecord(d.Key)
			if !ok {
				return nil, false
			}
			value, ok := l.fieldTypeRecord(d.Value)
			if !ok {
				return nil, false
			}
			rec.Field("key", ir.StorePtr.FType())
			rec.Field("value", ir.StorePtr.FType())
			vals = []ir.FieldVal{
				ir.Val("kind", ir.Lit(ir.Int(stdlib.KindDictionary))),
				ir.Val("key", ir.RelocInit(key).Plus(ir.Int(stdlib.MetadataOffset))),
				ir.Val("value", ir.RelocInit(value).Plus(ir.Int(stdlib.MetadataOffset))),
			}
			break
		}
		element, ok := l.fieldTypeRecord(u.(*types.Set).Elem)
		if !ok {
			return nil, false
		}
		rec.Field("element", ir.StorePtr.FType())
		vals = []ir.FieldVal{
			ir.Val("kind", ir.Lit(ir.Int(stdlib.KindSet))),
			ir.Val("element", ir.RelocInit(element).Plus(ir.Int(stdlib.MetadataOffset))),
		}
	case *types.Tuple:
		return l.tupleMetadata(info, u, rec, size, align)
	default:
		return nil, false
	}
	rows[vwInitWithTake] = fn(stdlib.WitnessTake)
	rows[vwGetEnumTag] = fn(stdlib.WitnessGetEnumTag)
	rows[vwStoreEnumTag] = fn(stdlib.WitnessStoreEnumTag)
	rows[vwSize] = ir.Lit(ir.Int(size))
	stride := alignUp(size, align)
	if stride == 0 {
		stride = 1
	}
	rows[vwStride] = ir.Lit(ir.Int(stride))
	if size > stdlib.ExistentialInline {
		flags |= stdlib.WitnessIsNonInline
	}
	rows[vwFlags] = ir.Lit(ir.Int(flags))
	vwt := l.out.Global(l.sym(info.Mangled+"WV"), ir.RO,
		ir.Array(vwWords, ir.StorePtr.FType())).Init(ir.List(rows...)).Align(8)

	vals = append([]ir.FieldVal{ir.Val("vwt", ir.RelocInit(vwt))}, vals...)
	g := l.out.Global(l.sym(info.Mangled+"Mf"), ir.RO, rec.FType()).
		Init(ir.Fields(vals...)).Align(8)
	l.meta[key] = g
	accessor := l.metadataAccessorName(info)
	_ = accessor
	l.metadataAccessorFor(info.Mangled+"Ma", g)
	return g, true
}

// tupleMetadata finishes a tuple's record, laid out as Swift lays tuple
// metadata out: how many elements, their labels, and each element's
// type and offset. Its value witnesses are a struct's with the same
// fields, so its references are retained and released as a struct's.
func (l *lowerer) tupleMetadata(info sil.TypeMetadata, tu *types.Tuple, rec *ir.Type, size, align int64) (*ir.Global, bool) {
	key := sil.StructuralKey(tu)
	image, ok := tupleImage(tu)
	if !ok {
		return nil, false
	}
	owned, ok := ownedWords(image, 0)
	if !ok {
		return nil, false
	}
	var vwt *ir.Global
	if len(owned) == 0 {
		vwt = l.valueWitnessTable(info.Mangled+"WV", size, align)
	} else {
		vwt = l.ownedValueWitnessTable(info, size, align, owned)
	}
	rec.Field("count", ir.StoreI64.FType())
	rec.Field("labels", ir.StorePtr.FType())
	vals := []ir.FieldVal{
		ir.Val("vwt", ir.RelocInit(vwt)),
		ir.Val("kind", ir.Lit(ir.Int(stdlib.KindTuple))),
		ir.Val("count", ir.Lit(ir.Int(int64(len(tu.Elements))))),
	}
	labelled := false
	var labels string
	for _, el := range tu.Elements {
		if el.Name != "" {
			labelled = true
		}
		labels += el.Name + " "
	}
	if labelled {
		vals = append(vals, ir.Val("labels", ir.RelocInit(l.cstring(info.Mangled+"ML", labels))))
	} else {
		vals = append(vals, ir.Val("labels", ir.Lit(ir.Int(0))))
	}
	for i, f := range image.Fields {
		record, ok := l.fieldTypeRecord(f.Type)
		if !ok {
			return nil, false
		}
		off, ok := types.Offsetof(image, f.Name, types.DefaultTarget64)
		if !ok {
			return nil, false
		}
		rec.Field("type"+itoa(i), ir.StorePtr.FType())
		rec.Field("offset"+itoa(i), ir.StoreI64.FType())
		vals = append(vals,
			ir.Val("type"+itoa(i), ir.RelocInit(record).Plus(ir.Int(stdlib.MetadataOffset))),
			ir.Val("offset"+itoa(i), ir.Lit(ir.Int(off))))
	}
	g := l.out.Global(l.sym(info.Mangled+"Mf"), ir.RO, rec.FType()).
		Init(ir.Fields(vals...)).Align(8)
	l.meta[key] = g
	l.metadataAccessorName(info)
	l.metadataAccessorFor(info.Mangled+"Ma", g)
	return g, true
}

// runtimeWitness is a value witness the runtime defines, as something a
// table row can point at.
func (l *lowerer) runtimeWitness(name string) ir.Symbol {
	if f, ok := l.runtimeFns[name]; ok {
		if s, isSym := f.(ir.Symbol); isSym {
			return s
		}
	}
	// The row is an address; the signature is never called through
	// here, so any declaration of the symbol serves.
	f := l.runtimeFunc(name, ir.NewSig().Param(ir.TypePtr))
	s, _ := f.(ir.Symbol)
	return s
}

// optionalEmptyCase says where an Optional writes its empty case: a tag
// byte after the payload holding 1, or the payload's own spare
// representation -- a null word for a reference or a pointer, the byte 2
// for a Bool.
func optionalEmptyCase(o *types.Optional) (offset, bytes, none int64, ok bool) {
	if _, tagged := optionalImage(o); tagged {
		return types.Sizeof(o.Wrapped, types.DefaultTarget64), 1, 1, true
	}
	// An enum of cases alone: nil is the first tag no case uses.
	if e, ok := tagOptional(o); ok {
		return 0, types.Sizeof(e, types.DefaultTarget64), int64(len(e.Cases)), true
	}
	switch u := o.Wrapped.Underlying().(type) {
	case *types.Basic:
		if u.Kind() == types.Bool {
			return 0, 1, 2, true
		}
		if u.Kind() == types.String {
			return 8, 8, 0, true
		}
		return 0, 0, 0, false
	case *types.Class, *types.Array, *types.Dictionary, *types.Set:
		return 0, 8, 0, true
	case *types.Struct, *types.Tuple:
		// A struct that spares a representation: its never-zero word zero.
		if _, at, ok := spareStructOptional(o); ok {
			return at, 8, 0, true
		}
	// An existential's empty case is a null metadata word, as Swift's is:
	// every value it holds has a type.
	case *types.Existential, *types.Protocol:
		return stdlib.ExistentialMetadata, 8, 0, true
	}
	if _, isPointer := machineOf(o.Wrapped); isPointer && types.Sizeof(o.Wrapped, types.DefaultTarget64) == 8 {
		r, _ := machineOf(o.Wrapped)
		if r.reg == ir.TypePtr {
			return 0, 8, 0, true
		}
	}
	return 0, 0, 0, false
}

// classMetadataFor emits a class's metadata record, once, where this
// module was asked for one, and yields it.
//
// A class value is one reference, so its value witnesses are the
// runtime's for a reference. The descriptor names the class and its
// module, which is what describing an instance writes; its fields are
// not described, as Swift does not describe them either.
func (l *lowerer) classMetadataFor(className string) (*ir.Global, bool) {
	info, ok := l.module.MetadataFor(className)
	if !ok {
		return nil, false
	}
	if _, isClass := info.Layout.Underlying().(*types.Class); !isClass {
		return nil, false
	}
	if l.meta == nil {
		l.meta = map[string]*ir.Global{}
	}
	if g, ok := l.meta[className]; ok {
		return g, g != nil
	}
	l.meta[className] = nil

	fn := func(name string) ir.Init { return ir.RelocInit(l.runtimeWitness(name)) }
	rows := make([]ir.Init, vwWords)
	rows[vwInitBufferWithCopyOfBuffer] = fn(stdlib.WitnessReferenceCopy)
	rows[vwDestroy] = fn(stdlib.WitnessReferenceDestroy)
	rows[vwInitWithCopy] = fn(stdlib.WitnessReferenceCopy)
	rows[vwAssignWithCopy] = fn(stdlib.WitnessReferenceAssignCopy)
	rows[vwInitWithTake] = fn(stdlib.WitnessTake)
	rows[vwAssignWithTake] = fn(stdlib.WitnessReferenceAssignTake)
	rows[vwGetEnumTag] = fn(stdlib.WitnessGetEnumTag)
	rows[vwStoreEnumTag] = fn(stdlib.WitnessStoreEnumTag)
	rows[vwSize] = ir.Lit(ir.Int(8))
	rows[vwStride] = ir.Lit(ir.Int(8))
	rows[vwFlags] = ir.Lit(ir.Int(7 | stdlib.WitnessIsNonPOD))
	vwt := l.out.Global(l.sym(info.Mangled+"WV"), ir.RO,
		ir.Array(vwWords, ir.StorePtr.FType())).Init(ir.List(rows...)).Align(8)

	descr := l.nominalDescriptorWith(info, &types.Struct{Name: info.Name}, nominalClassFlags)
	rec := l.out.Struct("meta_" + identSafe(className))
	rec.Field("vwt", ir.StorePtr.FType())
	rec.Field("kind", ir.StoreI64.FType())
	rec.Field("descriptor", ir.StorePtr.FType())
	g := l.out.Global(l.sym(info.Mangled+"Mf"), ir.RO, rec.FType()).
		Init(ir.Fields(
			ir.Val("vwt", ir.RelocInit(vwt)),
			ir.Val("kind", ir.Lit(ir.Int(stdlib.KindClass))),
			ir.Val("descriptor", ir.RelocInit(descr)),
		)).Align(8)
	g.Export()
	l.meta[className] = g
	l.metadataAccessorFor(info.Mangled+"Ma", g)
	return g, true
}

// witnessFlags is a value witness table's flags word: the alignment
// mask, whether copying the value is more than copying its bytes, and
// whether an existential holds it in a box rather than in its buffer.
func witnessFlags(size, align int64, nonPOD bool) int64 {
	flags := align - 1
	if nonPOD {
		flags |= stdlib.WitnessIsNonPOD
	}
	if size > stdlib.ExistentialInline {
		flags |= stdlib.WitnessIsNonInline
	}
	return flags
}

// enumDescriptor is an enum's nominal descriptor. Its fields are its
// cases, in the order of their tags -- the cases that carry a payload,
// then the ones that do not, as enumTagOf numbers them -- each with its
// name and its payload's metadata, or null for none. The last word, a
// struct's field offset vector, is instead where the tag starts: after
// the largest payload. The runtime reads the tag there and names the case.
func (l *lowerer) enumDescriptor(info sil.TypeMetadata, e *types.Enum) *ir.Global {
	name := l.sym(info.Mangled + "Mn")
	if g, ok := l.descriptors[name]; ok {
		return g
	}
	parent := l.moduleDescriptor(info)
	text := l.cstring(info.Mangled+"MnName", info.Name)
	g := l.out.Global(name, ir.RO, ir.Array(7, ir.StoreI32.FType()))
	g.Export()
	if l.descriptors == nil {
		l.descriptors = map[string]*ir.Global{}
	}
	l.descriptors[name] = g

	var ordered []*types.EnumCase
	for _, payload := range []bool{true, false} {
		for _, c := range e.Cases {
			if c != nil && (c.AssociatedType != nil) == payload {
				ordered = append(ordered, c)
			}
		}
	}
	rec := l.out.Struct("cases_" + identSafe(info.Type))
	rec.Field("count", ir.StoreI64.FType())
	vals := []ir.FieldVal{ir.Val("count", ir.Lit(ir.Int(int64(len(ordered)))))}
	for i, c := range ordered {
		caseName := l.cstring(info.Mangled+"MF"+itoa(i), c.Name)
		payload := ir.Init(ir.Lit(ir.Int(0)))
		if c.AssociatedType != nil {
			if r, ok := l.fieldTypeRecord(c.AssociatedType); ok {
				payload = ir.RelocInit(r)
				// An indirect case carries a box; the record's low bit
				// says so, and the runtime reads the value out of the
				// box, past its header.
				if c.Indirect {
					payload = ir.RelocInit(r).Plus(ir.Int(stdlib.FieldIndirect))
				}
			}
		}
		rec.Field("name"+itoa(i), ir.StorePtr.FType())
		rec.Field("type"+itoa(i), ir.StorePtr.FType())
		vals = append(vals, ir.Val("name"+itoa(i), ir.RelocInit(caseName)), ir.Val("type"+itoa(i), payload))
	}
	cases := l.out.Global(l.sym(info.Mangled+"MF"), ir.RO, rec.FType()).Init(ir.Fields(vals...)).Align(8)

	g.Init(ir.List(
		ir.Lit(ir.Int(nominalEnumFlags)),
		relative(parent, g, 1*4),
		relative(text, g, 2*4),
		relative(l.metadataAccessorName(info), g, 3*4),
		relative(cases, g, 4*4),
		ir.Lit(ir.Int(int64(len(ordered)))),
		ir.Lit(ir.Int(payloadArea(e))),
	))
	return g
}
