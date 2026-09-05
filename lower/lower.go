package lower

import (
	"fmt"
	"strings"

	"github.com/vertex-language/ir"
	"github.com/vertex-language/vsc/types"
	"github.com/vertex-language/vsc/vil"
)

// Options are what the caller decides about the translation.
type Options struct {
	// SymbolPrefix is what a name becomes in an object file: "_" on
	// Mach-O, empty on ELF and COFF.
	//
	// It is stated rather than read off the target, for the reason
	// vcc's lowering gives: the mapping belongs to the language and
	// not to the IR, nothing below this package renames a symbol, and
	// a platform that wants the underscore wants it on every symbol
	// the module defines or names. `swiftc -c` on Darwin writes
	// `_$s2m21fyS2iF` and `_main`, and a module built without the
	// prefix compiles and then fails to link, naming what it could
	// not find.
	SymbolPrefix string
}

// Module translates a lowered VIL module into a VIR module for the
// given target.
//
// The module must be in vil.StageLowered: this package translates the
// machine form of the program, and the ownership form is not it.
func Module(m *vil.Module, target ir.Target, opts Options) (*ir.Module, error) {
	if m.Stage() != vil.StageLowered {
		return nil, &Error{Err: ErrStage, What: string(m.Stage())}
	}
	l := &lowerer{
		out:    ir.NewModule(m.Name(), target),
		prefix: opts.SymbolPrefix,
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
	prefix string
	callee map[string]ir.Callee
	defs   map[string]*ir.Func

	// funcTypes are the VIR typedefs indirect calls name, one per
	// distinct Swift function type.
	funcTypes map[string]*ir.Type

	// The runtime's three, declared the first time something needs
	// one. What they do is runtime/'s business — it builds them as a
	// VIR module of its own — and that they exist is ours.
	retain  ir.Callee
	release ir.Callee
	alloc   ir.Callee
}

// sym is the object-file name of a VIL name.
//
// Every symbol goes through here — the module's own functions, the
// externals it calls, and the runtime's retain and release — because
// the prefix is the platform's and the platform does not care which
// of them it is looking at. The map from VIL name to VIR symbol is
// this function and nothing else, so a caller that wants to find a
// definition by its VIL name looks it up before this is applied.
func (l *lowerer) sym(name string) string { return l.prefix + name }

// declare gives every VIL function a VIR symbol before any body is
// filled, so a call to a function defined later has something to name.
func (l *lowerer) declare(f *vil.Func) error {
	sig, err := l.signature(f.Name(), f.Type())
	if err != nil {
		return err
	}
	if f.IsDeclaration() {
		l.callee[f.Name()] = l.out.ImportFunc(l.sym(f.Name()), sig)
		return nil
	}
	out := l.out.Func(l.sym(f.Name()))
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
		// A struct of more than one field is more than one register:
		// its memory image cut into words, which is what Swift passes.
		if n, ok := directWords(p.Type); ok {
			for i := 0; i < n; i++ {
				sig.Param(ir.TypeI64)
			}
			continue
		}
		r, ok := machine(p.Type)
		if !ok {
			return nil, &Error{Err: ErrType, Func: name, What: whyNoRegister(p.Type)}
		}
		sig.Param(r.reg)
	}
	for _, res := range t.Results {
		if empty(res.Type) {
			continue
		}
		if n, ok := directWords(res.Type); ok {
			for i := 0; i < n; i++ {
				sig.Ret(ir.TypeI64)
			}
			continue
		}
		r, ok := machine(res.Type)
		if !ok {
			return nil, &Error{Err: ErrType, Func: name, What: whyNoRegister(res.Type)}
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

// funcTypeOf is the VIR func typedef for a Swift function type, made
// once per distinct type and reused.
//
// The name is the type's own spelling with what VIR's identifiers do
// not admit taken out. It has to be a name because a callind names a
// typedef rather than carrying a signature inline; it has to be one
// name per signature because two typedefs with the same shape are two
// types.
func (l *lowerer) funcTypeOf(sig *types.Signature) (*ir.Type, error) {
	name := funcTypeName(sig)
	if t, ok := l.funcTypes[name]; ok {
		return t, nil
	}
	s := ir.NewSig()
	for _, p := range sig.Params {
		if n, ok := directWords(vil.Object(p.Type)); ok {
			for i := 0; i < n; i++ {
				s.Param(ir.TypeI64)
			}
			continue
		}
		r, ok := machineOf(p.Type)
		if !ok {
			return nil, fmt.Errorf("no register for %s", p.Type)
		}
		s.Param(r.reg)
	}
	if sig.Results != nil && !isVoidType(sig.Results) {
		if n, ok := directWords(vil.Object(sig.Results)); ok {
			for i := 0; i < n; i++ {
				s.Ret(ir.TypeI64)
			}
		} else {
			r, ok := machineOf(sig.Results)
			if !ok {
				return nil, fmt.Errorf("no register for %s", sig.Results)
			}
			s.Ret(r.reg)
		}
	}
	t := l.out.FuncType(name, s)
	if l.funcTypes == nil {
		l.funcTypes = map[string]*ir.Type{}
	}
	l.funcTypes[name] = t
	return t, nil
}

// funcTypeName is a VIR identifier standing for a Swift function type.
func funcTypeName(sig *types.Signature) string {
	var b strings.Builder
	b.WriteString("fn")
	for _, p := range sig.Params {
		b.WriteByte('_')
		b.WriteString(identSafe(p.Type.String()))
	}
	b.WriteString("__")
	if sig.Results != nil && !isVoidType(sig.Results) {
		b.WriteString(identSafe(sig.Results.String()))
	} else {
		b.WriteString("void")
	}
	return b.String()
}

// identSafe is a type's spelling with everything VIR's identifiers do
// not admit replaced, so that two different types cannot collide.
func identSafe(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('.')
		}
	}
	return b.String()
}

// isVoidType is the empty result, which VIR spells by having none.
func isVoidType(t types.Type) bool {
	b, ok := t.Underlying().(*types.Basic)
	return ok && (b.Kind() == types.Void)
}
