package lower

import (
	"github.com/vertex-language/ir"
	"github.com/vertex-language/vsc/types"
	"github.com/vertex-language/vsc/vil"
)

// Enums with a payload.
//
// A case may carry a value, and the cases share the space it goes in:
// `enum Shape { case dot; case line(Int32); case box(Int32, Int32) }`
// is nine bytes -- eight for the largest payload and one for the tag
// that says which case is there. swiftc's own bytes for it, read out
// of a value:
//
//	Shape.dot      = 0 0 0 0 0 0 0 0 2
//	Shape.line(7)  = 7 0 0 0 0 0 0 0 0
//	Shape.box(3,4) = 3 0 0 0 4 0 0 0 1
//
// So the payload sits at offset zero, whatever case it belongs to,
// and the tag byte follows the largest of them. The tags are the
// cases that carry something, in declaration order, and then the ones
// that carry nothing, also in declaration order -- which is why line
// is 0, box is 1 and dot is 2.
//
// Unlike a struct, an enum's fields overlap, so what crosses a call
// is its bytes rather than its fields. enumImage says that: the value
// travels as words, and composing and reading those words is the work
// below.

// enumImage is the struct a payload enum travels as: its bytes cut
// into words.
//
// Words rather than fields, because the cases share the payload area
// and there is no one list of fields to name. Everything the ABI asks
// -- how many registers, direct or indirect, how to pass and return
// -- is a question about the bytes, and this is the bytes.
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
func payloadEnumOf(t vil.Type) (*types.Enum, bool) {
	if !t.IsValid() || t.IsAddress() || t.Formal() == nil {
		return nil, false
	}
	e, ok := t.Formal().Underlying().(*types.Enum)
	if !ok || !hasPayload(e) {
		return nil, false
	}
	return e, true
}

// enumTagOf is the number a case is written as.
//
// The cases that carry something come first, in declaration order,
// then the ones that carry nothing. That is Swift's order and not an
// arbitrary one: swiftc writes 0 for line, 1 for box and 2 for dot in
// `enum Shape { case dot; case line(Int32); case box(Int32, Int32) }`,
// where dot is declared first and numbered last.
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
		if s := types.Sizeof(c.AssociatedType, types.DefaultTarget64); s > max {
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
	if _, ok := machineOf(t); !ok {
		return nil, false
	}
	return []leaf{{typ: t, offset: 0}}, true
}

// makePayloadEnum builds one case of an enum that has payloads: the
// payload's scalars placed at their offsets, and the tag after them.
func (c *fn) makePayloadEnum(in *vil.Inst, res *vil.Value, e *types.Enum) error {
	name := memberName(in.Aux().Member)
	tag, ok := enumTagOf(e, name)
	if !ok {
		return c.fail(ErrUnsupported, in.Op(), "no case "+name+" in "+e.Name)
	}
	var payload types.Type
	for _, k := range e.Cases {
		if k != nil && k.Name == name {
			payload = k.AssociatedType
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
		parts, ok := c.multi[in.Args()[0]]
		if !ok {
			got, err := c.operand(in, in.Args()[0])
			if err != nil {
				return err
			}
			parts = []ir.Value{got}
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
	// And the tag, after the largest payload.
	if err := c.placeInWords(in, words, c.b.I32.Const(tag),
		types.Typ[types.UInt8], payloadArea(e)); err != nil {
		return err
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
func (c *fn) placeInWords(in *vil.Inst, words []ir.I64, v ir.Value, t types.Type, off int64) error {
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
func (c *fn) readFromWords(in *vil.Inst, words []ir.Value, t types.Type, off int64) (ir.Value, error) {
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

// switchPayloadEnum branches on which case an enum holds and hands
// each arm what that case carries.
//
// The tag is the byte after the largest payload, so reading it is a
// read from the words like any other; the payload is read at offset
// zero, with the shape the case that arm belongs to declared. Each
// arm gets its own, which is why the payload is read per target
// rather than once.
func (c *fn) switchPayloadEnum(in *vil.Inst, e *types.Enum) error {
	v := in.Args()[0]
	words, ok := c.multi[v]
	if !ok {
		got, err := c.operand(in, v)
		if err != nil {
			return err
		}
		words = []ir.Value{got}
	}
	tagValue, err := c.readFromWords(in, words, types.Typ[types.UInt8], payloadArea(e))
	if err != nil {
		return err
	}
	tag, ok := tagValue.(ir.I32)
	if !ok {
		return c.fail(ErrType, in.Op(), "a tag this cannot compare")
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
		}
		targets[i] = dest.To(args...)
	}
	if dflt == nil {
		return c.fail(ErrUnsupported, in.Op(), "a switch with no default edge")
	}
	for i := range targets {
		if targets[i].Block() == nil {
			targets[i] = dflt.To()
		}
	}
	c.b.BrTable(tag, targets, dflt.To())
	return nil
}

// payloadArgs is what one case carries, read out of the words.
func (c *fn) payloadArgs(in *vil.Inst, words []ir.Value, e *types.Enum, name string) ([]ir.Value, error) {
	var payload types.Type
	for _, k := range e.Cases {
		if k != nil && k.Name == name {
			payload = k.AssociatedType
		}
	}
	if payload == nil {
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
	return out, nil
}
