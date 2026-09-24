package analyzer

import (
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/types"
)

// withUnsafeBytesCall types `a.withUnsafeBytes { raw in ... }` over an
// array: the closure is given an UnsafeRawBufferPointer over the bytes
// the elements are, and what it returns is what the call does. It
// reports false for any other call.
func (c *checker) withUnsafeBytesCall(mem *ast.MemberExpr, base types.Type, args []*ast.CallArg, expected types.Type, scope *Scope) (types.Type, bool) {
	if mem.Name == nil || len(args) != 1 || args[0].Label != nil || base == nil {
		return nil, false
	}
	// What the closure is given: an array's bytes, an array's elements
	// to write, or a String's bytes with a NUL after them.
	var param types.Type
	switch mem.Name.Text(c.file) {
	case "withUnsafeBytes":
		if _, ok := base.Underlying().(*types.Array); !ok && !c.isArraySlice(base) {
			return nil, false
		}
		tn, _ := scope.Lookup("UnsafeRawBufferPointer").(*TypeNameSymbol)
		if tn == nil || tn.Type() == nil {
			return nil, false
		}
		param = tn.Type()
	case "withUnsafeBufferPointer":
		var elem types.Type
		if arr, ok := base.Underlying().(*types.Array); ok {
			elem = arr.Elem
		} else if gi, ok := base.(*types.GenericInstance); ok && c.isArraySlice(base) && len(gi.Args) == 1 {
			elem = gi.Args[0]
		} else {
			return nil, false
		}
		tn, _ := scope.Lookup("UnsafeBufferPointer").(*TypeNameSymbol)
		if tn == nil || tn.Type() == nil {
			return nil, false
		}
		param = &types.GenericInstance{Base: tn.Type(), Args: []types.Type{elem}}
	case "withUnsafeMutableBufferPointer":
		arr, ok := base.Underlying().(*types.Array)
		if !ok {
			return nil, false
		}
		tn, _ := scope.Lookup("UnsafeMutableBufferPointer").(*TypeNameSymbol)
		if tn == nil || tn.Type() == nil {
			return nil, false
		}
		c.checkMutableReceiver(mem.X, mem.Name.Pos(), "cannot use mutating member on immutable value", scope)
		param = &types.GenericInstance{Base: tn.Type(), Args: []types.Type{arr.Elem}}
	case "withUnsafeMutableBytes":
		if _, ok := base.Underlying().(*types.Array); !ok {
			return nil, false
		}
		tn, _ := scope.Lookup("UnsafeMutableRawBufferPointer").(*TypeNameSymbol)
		if tn == nil || tn.Type() == nil {
			return nil, false
		}
		c.checkMutableReceiver(mem.X, mem.Name.Pos(), "cannot use mutating member on immutable value", scope)
		param = tn.Type()
	case "withCString":
		if b, ok := base.Underlying().(*types.Basic); !ok || b.Kind() != types.String {
			return nil, false
		}
		param = &types.Pointer{Elem: types.Typ[types.Int8]}
	default:
		return nil, false
	}
	// What the call is wanted to be is what the closure returns, which is
	// how a closure of several statements knows its result.
	want := &types.Signature{Params: []*types.Param{{Name: "raw", Type: param}}, Results: expected}
	got := c.checkExpr(args[0].X, want, scope)
	sig, ok := got.Underlying().(*types.Signature)
	if !ok || len(sig.Params) != 1 {
		c.typeErrorf(args[0].Pos(), "withUnsafeBytes takes a closure given the bytes: '(UnsafeRawBufferPointer) -> R'")
		return types.Typ[types.Invalid], true
	}
	var result types.Type = types.Typ[types.Void]
	if sig.Results != nil {
		result = sig.Results
	}
	c.info.Types[mem] = &types.Signature{Params: []*types.Param{{Type: got}}, Results: result}
	return result, true
}

// isCoreTask reports whether t is the core's Task, and not a type a
// program declared of the same name.
func (c *checker) isCoreTask(t types.Type) bool {
	if gi, ok := t.(*types.GenericInstance); ok {
		t = gi.Base
	}
	if t == nil || !c.info.CoreTypes[t] {
		return false
	}
	st, ok := t.Underlying().(*types.Struct)
	return ok && st.Name == "Task"
}

// taskResult is what a core Task's operation returns: the Success of
// Task<Success, Failure>, and nothing for a Task written without it.
func (c *checker) taskResult(t types.Type) types.Type {
	if gi, ok := t.(*types.GenericInstance); ok && len(gi.Args) > 0 {
		return gi.Args[0]
	}
	return types.Typ[types.Void]
}

// taskOf is core's Task for operations returning result: Task itself for
// those returning nothing, and an instance of it naming the result for the
// rest. One whose operation throws names the error too, `Task<T, any
// Error>`, and its value is read with try.
func taskOf(base, result types.Type, throws ...bool) types.Type {
	if len(throws) > 0 && throws[0] {
		if returnsNothing(result) {
			result = types.Typ[types.Void]
		}
		return &types.GenericInstance{Base: base, Args: []types.Type{result,
			&types.Existential{Protocols: []*types.Protocol{types.ErrorProtocol}}}}
	}
	if returnsNothing(result) {
		return base
	}
	return &types.GenericInstance{Base: base, Args: []types.Type{result}}
}

// taskOfArgs is core's Task spelled with its arguments, Task<Success,
// Failure>: a Failure other than Never makes one that throws.
func taskOfArgs(base types.Type, args []types.Type) types.Type {
	throws := len(args) > 1 && args[1] != nil && !types.Identical(args[1], types.Typ[types.Never])
	return taskOf(base, args[0], throws)
}

// TaskThrows reports whether a core Task's operation may throw, which
// makes reading its value a call that may.
func TaskThrows(t types.Type) bool {
	gi, ok := t.(*types.GenericInstance)
	return ok && len(gi.Args) > 1
}

// isArraySlice reports whether t is the core's ArraySlice of something.
func (c *checker) isArraySlice(t types.Type) bool {
	inst, ok := t.(*types.GenericInstance)
	if !ok || inst.Base == nil || !c.info.CoreTypes[inst.Base] {
		return false
	}
	st, ok := inst.Base.Underlying().(*types.Struct)
	return ok && st.Name == "ArraySlice"
}

// arraySliceOf is ArraySlice<elem>, as the core declares it, or nil.
func (c *checker) arraySliceOf(elem types.Type, scope *Scope) types.Type {
	tn, _ := scope.Lookup("ArraySlice").(*TypeNameSymbol)
	if tn == nil || tn.Type() == nil {
		return nil
	}
	return &types.GenericInstance{Base: tn.Type(), Args: []types.Type{elem}}
}

// rangeOf is the core's range of bound by name -- Range, which `a..<b`
// makes, ClosedRange, PartialRangeUpTo, ... -- or Invalid if the core
// has none.
func (c *checker) rangeOf(name string, bound types.Type) types.Type {
	var tn *TypeNameSymbol
	if core := c.modules["Swift"]; core != nil {
		tn, _ = core.Lookup(name).(*TypeNameSymbol)
	}
	if tn == nil || tn.Type() == nil {
		return types.Typ[types.Invalid]
	}
	return &types.GenericInstance{Base: tn.Type(), Args: []types.Type{bound}}
}

// returnsNothing reports whether a result is Void, or the empty tuple it
// is written as.
func returnsNothing(t types.Type) bool {
	if t == nil || types.Identical(t, types.Typ[types.Void]) {
		return true
	}
	tu, ok := t.Underlying().(*types.Tuple)
	return ok && len(tu.Elements) == 0
}
