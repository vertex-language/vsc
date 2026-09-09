package gen

import (
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/types"
	"github.com/vertex-language/vsc/vil"
)

// Unsafe pointers.
//
// A pointer here is an address and nothing else: types.Pointer is a
// kind rather than a struct, so a value of one is the machine's
// pointer register. Swift's is a struct around a Builtin.RawPointer,
// which is why swiftc reaches through it first --
//
//	%2 = struct_extract %0, #UnsafePointer._rawValue
//	%3 = pointer_to_address %2 to [strict] $*Int32
//	%4 = load %3
//
// -- and why this does not: there is no wrapper to reach through, and
// the value is already the thing struct_extract would have produced.
// Everything after that line is the same instruction in the same
// order.
//
// What a pointer can do here is what a C signature is made of: cross
// a call, be read and written through, be compared with another, be
// read as one of the other four spellings, and be made out of `&x`.
// What it cannot do is arithmetic, allocation, and `p[i]` -- the last
// wants a subscript, and the middle one is reachable through C's own
// malloc in the meantime. See the TODO.

// pointerOf is the unsafe pointer a type is, if it is one.
func pointerOf(t types.Type) (*types.Pointer, bool) {
	if t == nil {
		return nil, false
	}
	p, ok := t.Underlying().(*types.Pointer)
	return p, ok
}

// isPointee reports whether a member expression is `p.pointee` on a
// pointer, and hands back the pointer it is on.
func (g *gen) isPointee(e *ast.MemberExpr) (*types.Pointer, bool) {
	if e == nil || e.Name == nil || g.text(e.Name) != "pointee" {
		return nil, false
	}
	p, ok := pointerOf(g.typeOf(e.X))
	if !ok || !p.Dereferenceable() {
		return nil, false
	}
	return p, true
}

// pointeeAddr is the address `p.pointee` names: the pointer, read as
// an address of the element type.
//
// `[strict]` in SIL says the memory really is that type rather than
// bytes being reinterpreted, which is what a typed pointer promises
// and what makes the load a load rather than a bitcast.
func (g *gen) pointeeAddr(e *ast.MemberExpr, p *types.Pointer) *vil.Value {
	v := g.rvalue(e.X)
	if v == nil {
		return nil
	}
	return g.blk.PointerToAddress(v, lowerType(p.Elem).Address())
}

// pointeeRead lowers a read through a pointer.
func (g *gen) pointeeRead(e *ast.MemberExpr, p *types.Pointer) *vil.Value {
	addr := g.pointeeAddr(e, p)
	if addr == nil {
		return nil
	}
	t := lowerType(p.Elem)
	// No access scope. An exclusivity check is a claim about storage
	// the compiler knows the extent of, and the whole of what an
	// unsafe pointer says is that it does not know: the memory may be
	// C's, and nothing here may assume who else is holding it.
	return g.loaded(g.blk.Load(addr, loadQualifier(t)), t)
}

// pointeeAddrForWrite is the address a write through a pointer goes
// to, and says why where there is none.
//
// Only a mutable pointer may be written through. `UnsafePointer<T>`
// is Swift's `const T *` and the checker treats `p.pointee` on one as
// a get-only property, which is where the refusal belongs -- but a
// write that reached here anyway must not be lowered, because the
// address is perfectly good and the store would happen.
func (g *gen) pointeeAddrForWrite(e *ast.MemberExpr, p *types.Pointer) *vil.Value {
	if !p.Mutable {
		g.errorAt(e, "cannot write through '"+p.String()+"': it points at "+
			"something it may only read. 'UnsafeMutablePointer' is the one "+
			"that writes")
		return nil
	}
	return g.pointeeAddr(e, p)
}

// pointerConvert lowers `UnsafeRawPointer(p)` and its neighbours, and
// reports whether the call was one.
//
// Every one of these is the same address read as something else, so
// there is no instruction: the value crosses unchanged and only its
// static type differs. Swift's are initializers on a struct wrapping
// a Builtin.RawPointer, and swiftc's own code for them is the extract
// and the re-wrap that cancel out.
//
// Both directions are lowered and neither is checked, because there
// is nothing here to check. Going from a typed pointer to a raw one
// loses the element and is always sound; going the other way is a
// claim about memory nobody verified, which is what "unsafe" means
// and what Swift allows too.
//
// The one refusal is an optional source. Reading `p` as another
// pointer where `p` may be null has to answer something that may be
// null, and which optional that is belongs to the checker, which does
// not decide it yet.
func (g *gen) pointerConvert(e *ast.CallExpr, arg ast.Expr, from, to types.Type) (*vil.Value, bool) {
	if _, ok := pointerOf(to); !ok {
		return nil, false
	}
	// The source has to be a pointer too. `UnsafeRawPointer(7)` is
	// not a conversion -- Swift has no such initializer -- and
	// leaving it to fall through would make it a construction of a
	// type that has no fields.
	if _, ok := pointerOf(from); !ok {
		// An optional pointer is a pointer that may be null, and
		// reading one as another keeps that: the null stays null,
		// because null is what the empty case is. It is refused
		// rather than lowered only because the result's optionality
		// is the checker's to decide and it does not decide it yet.
		if o, isOpt := optionalOf(from); isOpt {
			if _, wraps := pointerOf(o.Wrapped); wraps {
				g.refuse(e, "converting an optional pointer, which would have to "+
					"answer an optional and does not say so yet")
				return nil, true
			}
		}
		return nil, false
	}
	return g.rvalue(arg), true
}

// pointerEquality lowers `p == q` and `p != q`, and reports whether
// the expression was one.
//
// core declares no operator over a pointer -- what would it be an
// operator on, when the five spellings are one kind here -- so there
// is no symbol to resolve and this is reached from the branch where
// there is none. The same place `??` and an enum's `==` are reached
// from, and for the same reason.
//
// Two addresses are equal when they are the same place. That is one
// instruction, and swiftc emits the same one.
func (g *gen) pointerEquality(e *ast.BinaryExpr, op string) (*vil.Value, bool) {
	if op != "==" && op != "!=" {
		return nil, false
	}
	if _, ok := pointerOf(g.typeOf(e.X)); !ok {
		return nil, false
	}
	if _, ok := pointerOf(g.typeOf(e.Y)); !ok {
		return nil, false
	}
	lhs, rhs := g.rvalue(e.X), g.rvalue(e.Y)
	if lhs == nil || rhs == nil {
		return nil, true
	}
	verb := "cmp_eq_RawPointer"
	if op == "!=" {
		verb = "cmp_ne_RawPointer"
	}
	bit := g.blk.Builtin(verb, vil.Object(vil.BuiltinInt1), lhs, rhs)
	return g.boolOf(bit), true
}
