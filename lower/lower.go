package lower

import (
	"github.com/vertex-language/ir"
	"github.com/vertex-language/vsc/vil"
)

// Module translates a lowered VIL module into a VIR module for the
// given target.
//
// The module must be in vil.StageLowered: this package translates the
// machine form of the program, and the ownership form is not it.
func Module(m *vil.Module, target ir.Target) (*ir.Module, error) {
	if m.Stage() != vil.StageLowered {
		return nil, &Error{Err: ErrStage, What: string(m.Stage())}
	}
	l := &lowerer{
		out:    ir.NewModule(m.Name(), target),
		callee: make(map[string]ir.Callee),
		defs:   make(map[string]*ir.Func),
	}
	for _, f := range m.Funcs() {
		if err := l.declare(f); err != nil {
			return nil, err
		}
	}
	for _, f := range m.Funcs() {
		if f.IsDeclaration() {
			continue
		}
		if err := l.define(f); err != nil {
			return nil, err
		}
	}
	if err := l.out.Err(); err != nil {
		return nil, &Error{Err: ErrIR, What: err.Error()}
	}
	return l.out, nil
}

// A lowerer holds what is shared across the whole module.
type lowerer struct {
	out    *ir.Module
	callee map[string]ir.Callee
	defs   map[string]*ir.Func

	// The retain and release the runtime provides, declared the first
	// time a retain or a release needs them. What they do is vcc's
	// business; that they exist is ours.
	retain  ir.Callee
	release ir.Callee
}

// declare gives every VIL function a VIR symbol before any body is
// filled, so a call to a function defined later has something to name.
func (l *lowerer) declare(f *vil.Func) error {
	sig, err := l.signature(f.Name(), f.Type())
	if err != nil {
		return err
	}
	if f.IsDeclaration() {
		l.callee[f.Name()] = l.out.ImportFunc(f.Name(), sig)
		return nil
	}
	out := l.out.Func(f.Name())
	l.defs[f.Name()] = out
	if err := l.applySig(f, out, sig); err != nil {
		return err
	}
	// Only a public symbol leaves the object file. Package linkage is
	// visible to the modules built alongside this one, which is a
	// question for whoever links them and not one an object file can
	// answer, so it is internal here too.
	switch f.Linkage() {
	case vil.Public, vil.PublicExternal:
		out.Export()
	default:
		out.Internal()
	}
	l.callee[f.Name()] = out
	return nil
}

// signature is the VIR calling signature of a lowered VIL function
// type. Conventions do not survive: @owned and @guaranteed say who
// releases the value, and by this point the retains and releases are
// already written down as instructions.
func (l *lowerer) signature(name string, t *vil.FuncType) (*ir.Sig, error) {
	sig := ir.NewSig()
	for _, p := range t.Params {
		r, ok := machine(p.Type)
		if !ok {
			return nil, &Error{Err: ErrType, Func: name, What: p.Type.String()}
		}
		sig.Param(r.reg)
	}
	for _, res := range t.Results {
		if empty(res.Type) {
			continue
		}
		r, ok := machine(res.Type)
		if !ok {
			return nil, &Error{Err: ErrType, Func: name, What: res.Type.String()}
		}
		sig.Ret(r.reg)
	}
	if t.ErrorType.IsValid() {
		return nil, &Error{Err: ErrUnsupported, Func: name, What: "a throwing function"}
	}
	if t.Async {
		return nil, &Error{Err: ErrUnsupported, Func: name, What: "an async function"}
	}
	return sig, nil
}

// applySig states the signature again through the calls that also
// create the function's parameter registers, which is how ir.Func
// wants it said. ir has one method per reg-type on purpose, so a verb
// it does not have is a compile error rather than a refusal at run
// time; the price is this switch.
func (l *lowerer) applySig(f *vil.Func, out *ir.Func, sig *ir.Sig) error {
	for i, p := range sig.Params() {
		name := "a" + itoa(i)
		switch p.Type {
		case ir.TypeI1:
			out.ParamI1(name)
		case ir.TypeI32:
			out.ParamI32(name)
		case ir.TypeI64:
			out.ParamI64(name)
		case ir.TypeF32:
			out.ParamF32(name)
		case ir.TypeF64:
			out.ParamF64(name)
		case ir.TypePtr:
			out.ParamPtr(name)
		default:
			return &Error{Err: ErrType, Func: f.Name(), What: p.Type.String()}
		}
	}
	for _, r := range sig.Rets() {
		switch r.Type {
		case ir.TypeI1:
			out.ReturnsI1()
		case ir.TypeI32:
			out.ReturnsI32()
		case ir.TypeI64:
			out.ReturnsI64()
		case ir.TypeF32:
			out.ReturnsF32()
		case ir.TypeF64:
			out.ReturnsF64()
		case ir.TypePtr:
			out.ReturnsPtr()
		default:
			return &Error{Err: ErrType, Func: f.Name(), What: r.Type.String()}
		}
	}
	return nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
