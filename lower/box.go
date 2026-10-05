package lower

import (
	"fmt"

	"github.com/vertex-language/ir"
	"github.com/vertex-language/vsc/internal/sil"
	"github.com/vertex-language/vsc/stdlib"
	"github.com/vertex-language/vsc/types"
)

// Boxes: the heap storage of a variable a closure captures, which the
// closure and the scope it was declared in share. Only a box something
// captures reaches lowering -- the box pass makes every other one a stack
// slot -- and it is a heap object with the value after its header and a
// destroyer that lets the value go.

// boxValue is where a box's value is, past the header.
const boxValue = stdlib.HeaderBytes

func (c *fn) allocBox(in *sil.Inst) error {
	res := in.Result()
	if res == nil {
		return nil
	}
	box, ok := in.Aux().Type.Formal().(*sil.BoxType)
	if !ok || box.Elem() == nil {
		return c.fail(ErrType, in.Op(), "a box with no element type")
	}
	elem := box.Elem()
	size := types.Sizeof(elem, types.DefaultTarget64)
	align := types.Alignof(elem, types.DefaultTarget64)
	if size < 0 || align <= 0 || align > boxValue {
		return c.fail(ErrUnsupported, in.Op(), "a box of "+elem.String())
	}
	if size == 0 {
		size = 1
	}
	meta, err := c.l.boxMetadata(elem)
	if err != nil {
		return c.fail(ErrUnsupported, in.Op(), err.Error())
	}
	if c.l.alloc == nil {
		c.l.alloc = c.l.out.ImportFunc(c.l.sym(stdlib.Alloc),
			ir.NewSig().Param(ir.TypeI64).Ret(ir.TypePtr)).NoUnwind()
	}
	got := c.b.Call(c.l.alloc, c.b.I64.Const(size))
	if got.Len() == 0 {
		return c.fail(ErrIR, in.Op(), "the allocator returned nothing")
	}
	obj, ok := got.Value(0).(ir.Ptr)
	if !ok {
		return c.fail(ErrType, in.Op(), "the allocator did not return a pointer")
	}
	c.b.Ptr.Store(c.b.Ptr.GetAddr(meta), obj)
	c.def(res, obj)
	return nil
}

func (c *fn) projectBox(in *sil.Inst) error {
	res := in.Result()
	if res == nil {
		return nil
	}
	got, err := c.operand(in, in.Args()[0])
	if err != nil {
		return err
	}
	obj, ok := got.(ir.Ptr)
	if !ok {
		return c.fail(ErrType, in.Op(), "a box that is not a reference")
	}
	c.def(res, c.b.Ptr.Add(obj, c.b.I64.Const(boxValue)))
	return nil
}

// boxMetadata is the HeapMetadata of a box holding elem: a destroyer that
// releases what the value holds, then frees the box.
func (l *lowerer) boxMetadata(elem types.Type) (*ir.Global, error) {
	key := elem.String()
	if g, ok := l.boxes[key]; ok {
		return g, nil
	}
	one := &types.Struct{Fields: []*types.Field{{Name: "value", Type: elem}}}
	owned, ok := ownedWords(one, boxValue)
	if !ok {
		return nil, fmt.Errorf("a box of %s, whose copy is not a retain", elem)
	}
	f := l.out.Func(l.sym("$sVSCdestroybox_" + identSafe(key)))
	f.Internal()
	obj := f.ParamPtr("box")
	b := f.Entry()
	release := l.runtimeFunc(stdlib.Release, ir.NewSig().Param(ir.TypePtr))
	releaseString := l.runtimeFunc(stdlib.StringRelease, ir.NewSig().Param(ir.TypePtr))
	b = countOwned(f, b, obj, owned, releaseString, release, l.existentialCounter(false), "d")
	b.Call(l.runtimeFunc(stdlib.Dealloc, ir.NewSig().Param(ir.TypePtr)), obj)
	b.Return()
	g := l.out.Global(l.sym("$sVSCbox_"+identSafe(key)), ir.RO, ir.Array(2, ir.StorePtr.FType())).
		Init(ir.List(ir.RelocInit(f), ir.Lit(ir.Int(0)))).Align(8)
	if l.boxes == nil {
		l.boxes = map[string]*ir.Global{}
	}
	l.boxes[key] = g
	return g, nil
}
