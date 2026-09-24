package lower

import (
	"github.com/vertex-language/ir"
	"github.com/vertex-language/vsc/internal/sil"
	"github.com/vertex-language/vsc/types"
)

// Payload enum lowering: shared payload area at offset zero followed by the tag byte.

// enumImage returns a synthetic struct of i64 words representing the enum's raw bytes.
func enumImage(e *types.Enum) (*types.Struct, bool) {
	if e == nil || len(e.Cases) == 0 || !hasPayload(e) {
		return nil, false
	}
	size := types.Sizeof(e, types.DefaultTarget64)
	if size <= 0 {
		return nil, false
	}
	n := int((size + 7) / 8)
	fields := make([]*types.Field, 0, n)
	for i := 0; i < n; i++ {
		fields = append(fields, &types.Field{
			Name: "w" + itoa(i),
			Type: types.Typ[types.Int64],
		})
	}
	return &types.Struct{Name: e.Name, Fields: fields}, true
}

// hasPayload reports whether any of an enum's cases carries a value.
// One where none does is a tag and nothing else, and is lowered as
// the integer it is.
func hasPayload(e *types.Enum) bool {
	for _, c := range e.Cases {
		if c != nil && c.AssociatedType != nil {
			return true
		}
	}
	return false
}

// payloadEnumOf is the payload enum a type is, if it is one.
func payloadEnumOf(t sil.Type) (*types.Enum, bool) {
	if !t.IsValid() || t.IsAddress() || t.Formal() == nil {
		return nil, false
	}
	e, ok := t.Formal().Underlying().(*types.Enum)
	if !ok || !hasPayload(e) {
		return nil, false
	}
	return e, true
}

// enumTagOf returns the integer tag for an enum case (payload cases first, then empty cases).
func enumTagOf(e *types.Enum, name string) (int64, bool) {
	var n int64
	for _, c := range e.Cases {
		if c == nil || c.AssociatedType == nil {
			continue
		}
		if c.Name == name {
			return n, true
		}
		n++
	}
	for _, c := range e.Cases {
		if c == nil || c.AssociatedType != nil {
			continue
		}
		if c.Name == name {
			return n, true
		}
		n++
	}
	return 0, false
}

// payloadOffset is where the tag byte sits: after the largest
// payload, which is where swiftc puts it.
func payloadArea(e *types.Enum) int64 {
	var max int64
	for _, c := range e.Cases {
		if c == nil || c.AssociatedType == nil {
			continue
		}
		if s := types.Sizeof(sil.CaseStorage(c), types.DefaultTarget64); s > max {
			max = s
		}
	}
	return max
}

// caseLeaves is the scalars a case's payload is made of, with the
// offset each sits at inside the enum -- which is its offset inside
// the payload, because the payload starts at zero.
func caseLeaves(t types.Type) ([]leaf, bool) {
	if t == nil {
		return nil, true
	}
	// A payload of no bytes -- a one-case enum, an empty struct -- has
	// nothing to place.
	if types.Sizeof(t, types.DefaultTarget64) == 0 {
		return nil, true
	}
	if st, ok := t.Underlying().(*types.Struct); ok {
		return structLeaves(st)
	}
	if tu, ok := t.Underlying().(*types.Tuple); ok {
		st, ok := tupleImage(tu)
		if !ok {
			return nil, false
		}
		return structLeaves(st)
	}
	// A String and a function value are two words each.
	switch u := t.Underlying().(type) {
	case *types.Basic:
		if u.Kind() == types.String {
			return structLeaves(&types.Struct{Fields: []*types.Field{{Name: "payload", Type: t}}})
		}
	case *types.Signature:
		return structLeaves(&types.Struct{Fields: []*types.Field{{Name: "payload", Type: t}}})
	}
	// Another payload enum is carried as its words, the last holding its
	// tag, so those words are its leaves.
	if e, ok := t.Underlying().(*types.Enum); ok && hasPayload(e) {
		image, ok := enumImage(e)
		if !ok {
			return nil, false
		}
		return structLeaves(image)
	}
	if _, ok := machineOf(t); !ok {
		// Anything else laid out as a struct -- an optional with a tag
		// byte, one of a String or a function -- is its leaves.
		if st, ok := structOf(sil.Object(t)); ok {
			return structLeaves(st)
		}
		return nil, false
	}
	return []leaf{{typ: t, offset: 0}}, true
}

// makePayloadEnum builds one case of an enum that has payloads: the
// payload's scalars placed at their offsets, and the tag after them.
func (c *fn) makePayloadEnum(in *sil.Inst, res *sil.Value, e *types.Enum) error {
	name := memberName(in.Aux().Member)
	tag, ok := enumTagOf(e, name)
	if !ok {
		return c.fail(ErrUnsupported, in.Op(), "no case "+name+" in "+e.Name)
	}
	var payload types.Type
	for _, k := range e.Cases {
		if k != nil && k.Name == name {
			payload = sil.CaseStorage(k)
		}
	}
	ls, ok := caseLeaves(payload)
	if !ok {
		return c.fail(ErrUnsupported, in.Op(),
			"a case whose payload this cannot take apart: "+payload.String())
	}

	image, ok := enumImage(e)
	if !ok {
		return c.fail(ErrUnsupported, in.Op(), e.Name)
	}
	words := make([]ir.I64, len(image.Fields))
	for i := range words {
		words[i] = c.b.I64.Const(0)
	}

	// The payload's scalars, each at its own offset from the start.
	if len(ls) > 0 {
		arg := in.Args()[0]
		parts, ok := c.parts(arg)
		if !ok {
			if from, inMemory := c.mem[arg]; inMemory {
				// A payload too wide for registers is read out of its
				// storage a leaf at a time, as a field of one is.
				parts = make([]ir.Value, 0, len(ls))
				for _, l := range ls {
					r, ok := machineOf(l.typ)
					if !ok {
						return c.fail(ErrType, in.Op(), whyNoRegister(sil.Object(l.typ)))
					}
					v, err := c.loadScalar(in, c.fieldAddr(from, l.offset), r)
					if err != nil {
						return c.fail(ErrType, in.Op(), err.Error())
					}
					parts = append(parts, v)
				}
			} else {
				got, err := c.operand(in, arg)
				if err != nil {
					return err
				}
				parts = []ir.Value{got}
			}
		}
		if len(parts) != len(ls) {
			return c.fail(ErrUnsupported, in.Op(),
				"a payload whose registers do not match its scalars")
		}
		for i, l := range ls {
			if err := c.placeInWords(in, words, parts[i], l.typ, l.offset); err != nil {
				return err
			}
		}
	}
	// And the tag, after the largest payload -- unless there is one case,
	// which needs none: the enum is its payload, as types.Sizeof lays it
	// out and as swiftc does.
	if len(e.Cases) > 1 {
		if err := c.placeInWords(in, words, c.b.I32.Const(tag),
			types.Typ[types.UInt8], payloadArea(e)); err != nil {
			return err
		}
	}

	// Too wide for registers, it is written into the storage set aside
	// for it, and is that storage from then on.
	if slot, ok := c.wide[res]; ok {
		for i, w := range words {
			c.b.I64.Store(w, c.fieldAddr(slot, int64(i)*8))
		}
		c.mem[res] = slot
		return nil
	}
	out := make([]ir.Value, len(words))
	for i, w := range words {
		out[i] = w
	}
	if len(out) == 1 {
		c.def(res, out[0])
		return nil
	}
	c.multi[res] = out
	return nil
}

// placeInWords ors one scalar into the words at a byte offset.
func (c *fn) placeInWords(in *sil.Inst, words []ir.I64, v ir.Value, t types.Type, off int64) error {
	width := uint(types.Sizeof(t, types.DefaultTarget64) * 8)
	if width == 0 || width > 64 {
		return c.fail(ErrUnsupported, in.Op(), "a payload scalar of "+itoa(int(width))+" bits")
	}
	i := int(off / 8)
	shift := uint((off % 8) * 8)
	if i >= len(words) {
		return c.fail(ErrUnsupported, in.Op(), "a payload that does not fit the enum")
	}
	if shift+width > 64 {
		return c.fail(ErrUnsupported, in.Op(), "a payload scalar that straddles two words")
	}
	bits, ok := c.toWord(v, width)
	if !ok {
		return c.fail(ErrType, in.Op(), "a payload scalar this cannot widen")
	}
	if shift != 0 {
		bits = c.b.I64.Shl(bits, c.b.I64.Const(int64(shift)))
	}
	words[i] = c.b.I64.Or(words[i], bits)
	return nil
}

// readFromWords takes one scalar back out.
func (c *fn) readFromWords(in *sil.Inst, words []ir.Value, t types.Type, off int64) (ir.Value, error) {
	width := uint(types.Sizeof(t, types.DefaultTarget64) * 8)
	i := int(off / 8)
	shift := uint((off % 8) * 8)
	if i >= len(words) || width == 0 || shift+width > 64 {
		return nil, c.fail(ErrUnsupported, in.Op(), "a payload this cannot read back")
	}
	w, ok := words[i].(ir.I64)
	if !ok {
		return nil, c.fail(ErrType, in.Op(), "an enum word that is not a word")
	}
	if shift != 0 {
		w = c.b.I64.UShr(w, c.b.I64.Const(int64(shift)))
	}
	got, ok := c.fromWord(w, t, width)
	if !ok {
		return nil, c.fail(ErrType, in.Op(), "a payload scalar this cannot narrow")
	}
	return got, nil
}

// switchPayloadEnum lowers pattern matching over a payload enum into a jump table branch.
func (c *fn) switchPayloadEnum(in *sil.Inst, e *types.Enum) error {
	// Its words, from registers or -- an enum too wide for them -- from
	// the memory it is held in.
	v := in.Args()[0]
	words, werr := c.enumWords(in, v, e)
	if werr != nil {
		return werr
	}
	// A one-case enum has no tag to read: it is always that case.
	var tag ir.I32
	var err error
	if len(e.Cases) > 1 {
		var tagValue ir.Value
		tagValue, err = c.readFromWords(in, words, types.Typ[types.UInt8], payloadArea(e))
		if err != nil {
			return err
		}
		t, ok := tagValue.(ir.I32)
		if !ok {
			return c.fail(ErrType, in.Op(), "a tag this cannot compare")
		}
		tag = t
	} else {
		tag = c.b.I32.Const(0)
	}

	targets := make([]ir.BlockTarget, len(e.Cases))
	var dflt *ir.Block
	for _, k := range in.Aux().Cases {
		dest, ok := c.blocks[k.Dest]
		if !ok {
			return c.fail(ErrUnsupported, in.Op(), "no such block")
		}
		if k.Member == "" {
			dflt = dest
			continue
		}
		name := memberName(k.Member)
		i, ok := enumTagOf(e, name)
		if !ok || int(i) >= len(targets) {
			return c.fail(ErrUnsupported, in.Op(), "no case "+name+" in "+e.Name)
		}
		// An arm that did not bind what the case carries takes no
		// arguments: `case .line:` matches without naming the value,
		// and passing one to a block that declares none is a branch
		// the verifier refuses.
		var args []ir.Value
		if len(k.Dest.Args()) > 0 {
			args, err = c.payloadArgs(in, words, e, name)
			if err != nil {
				return err
			}
		} else if payload := casePayload(e, name); payload != nil && !sil.Object(payload).Trivial() {
			edge, err := c.dropEdge(in, words, payload, dest)
			if err != nil {
				return err
			}
			targets[i] = edge.To()
			continue
		}
		targets[i] = dest.To(args...)
	}
	if dflt == nil {
		return c.fail(ErrUnsupported, in.Op(), "a switch with no default edge")
	}
	// A default that takes the enum back -- OSSA's switch_enum hands its
	// operand to the default edge when that edge says so -- takes it
	// whole, and lets go of it itself, whichever case it caught.
	if dfltArgs := c.defaultTakes(in); dfltArgs {
		whole := c.args([]*sil.Value{v})
		for i := range targets {
			if targets[i].Block() == nil {
				targets[i] = dflt.To(whole...)
			}
		}
		c.b.BrTable(tag, targets, dflt.To(whole...))
		return nil
	}
	for i := range targets {
		if targets[i].Block() != nil {
			continue
		}
		// The default takes nothing, so a case it catches lets go of
		// what that case carries.
		if payload := casePayload(e, caseNameOf(e, int64(i))); payload != nil && !sil.Object(payload).Trivial() {
			edge, err := c.dropEdge(in, words, payload, dflt)
			if err != nil {
				return err
			}
			targets[i] = edge.To()
			continue
		}
		targets[i] = dflt.To()
	}
	c.b.BrTable(tag, targets, dflt.To())
	return nil
}

// defaultTakes reports whether a switch's default edge declares the enum
// switched on as its argument.
func (c *fn) defaultTakes(in *sil.Inst) bool {
	for _, k := range in.Aux().Cases {
		if k.Member == "" && k.Dest != nil {
			return len(k.Dest.Args()) == 1
		}
	}
	return false
}

// casePayload is what the case named name carries, or nil.
func casePayload(e *types.Enum, name string) types.Type {
	for _, k := range e.Cases {
		if k != nil && k.Name == name {
			return sil.CaseStorage(k)
		}
	}
	return nil
}

// caseNameOf is the name of the case whose tag is tag, or "".
func caseNameOf(e *types.Enum, tag int64) string {
	for _, k := range e.Cases {
		if k == nil {
			continue
		}
		if t, ok := enumTagOf(e, k.Name); ok && t == tag {
			return k.Name
		}
	}
	return ""
}

// payloadArgs is what one case carries, read out of the words.
func (c *fn) payloadArgs(in *sil.Inst, words []ir.Value, e *types.Enum, name string) ([]ir.Value, error) {
	var payload types.Type
	for _, k := range e.Cases {
		if k != nil && k.Name == name {
			payload = sil.CaseStorage(k)
		}
	}
	if payload == nil {
		return nil, nil
	}
	// A payload of no bytes held no bits, but the arm still names it: a
	// one-case enum is its tag in a register, which is zero.
	if types.Sizeof(payload, types.DefaultTarget64) == 0 {
		if _, ok := machineOf(payload); ok {
			z, err := c.zeroOf(in, payload)
			if err != nil {
				return nil, err
			}
			return []ir.Value{z}, nil
		}
		return nil, nil
	}
	ls, ok := caseLeaves(payload)
	if !ok {
		return nil, c.fail(ErrUnsupported, in.Op(),
			"a case whose payload this cannot take apart: "+payload.String())
	}
	out := make([]ir.Value, 0, len(ls))
	for _, l := range ls {
		got, err := c.readFromWords(in, words, l.typ, l.offset)
		if err != nil {
			return nil, err
		}
		out = append(out, got)
	}
	// A payload enum's last word ends at its tag, and past it sits this
	// enum's own tag. Those bytes are not the payload's: left in, they
	// would be or-ed into the tag of whatever the payload is put in next.
	if e, ok := payload.Underlying().(*types.Enum); ok && hasPayload(e) && len(out) > 0 {
		if rem := types.Sizeof(e, types.DefaultTarget64) % 8; rem != 0 {
			last, ok := out[len(out)-1].(ir.I64)
			if !ok {
				return nil, c.fail(ErrType, in.Op(), "an enum word that is not a word")
			}
			out[len(out)-1] = c.b.I64.And(last, c.b.I64.Const(int64(uint64(1)<<(uint(rem)*8)-1)))
		}
	}
	return out, nil
}
