// Package vil implements VIL: the ownership intermediate language, a
// clone of Swift's SIL.
//
// It sits between the checked tree and VIR. The tree knows what the
// program says; VIR knows what the machine does; VIL is where the
// language's own decisions are made and checked — where a value is
// owned or borrowed, where its lifetime ends, whether it was
// initialized before it was read, and where a retain belongs.
//
// # Cloned, deliberately
//
// The instructions carry Swift's names, the text form is Swift's, and
// the ownership model is Swift's. That is the point rather than a
// shortcut: `swiftc -emit-silgen` prints the answer to every question
// lowering has to decide, so a faithful clone turns a reference you
// read into a test you run.
//
//	swiftc -emit-silgen f.swift
//	vsc build --emit vil f.vs
//
// A construct Vertex does not have is not renamed, it is simply never
// emitted; a construct it grows arrives under Swift's name for it.
//
// # Two stages
//
// A module is raw or canonical, and the difference is what has been
// proved about it. StageRaw is what vil/gen emits: ownership is
// written down but nothing has been checked, and the program may
// still be rejected. StageCanonical is what vil/pass produces and
// what lower consumes: definite initialization has run, the ownership
// rules have been verified, and ARC is in place.
//
// # The two rules
//
// Everything else in this package exists to make these checkable:
//
//  1. An owned value is consumed exactly once on every path out of
//     its definition.
//  2. A guaranteed value is used only within the borrow scope that
//     produced it.
//
// # Held to it
//
// Three tests, and they check different things. vil/text's form tests
// hold every instruction that can be emitted to the text SIL writes
// for it. vil/verify's failure corpus holds the rules, one case per
// rule. And vil/gen's corpus takes programs through both compilers
// and requires the output to agree, which is the only one of the
// three that can catch this package and Swift drifting apart.
//
// # Shape
//
// A Module holds Funcs, Globals, VTables and WitnessTables. A Func
// holds Blocks; a Block holds arguments and Insts; an Inst has
// operands and results. Every operand and result is a *Value with a
// Type and an Ownership. Blocks take arguments rather than phi nodes,
// which is Swift's choice and VIR's too.
//
//	m := vil.NewModule("app", vil.StageRaw)
//	fn := m.Func("foo").SetLinkage(vil.Hidden).SetOSSA(true)
//	x := fn.Param(vil.Object(types.Typ[types.Int]), vil.Guaranteed)
//	fn.SetResult(vil.Object(types.Typ[types.Int]))
//
//	bb := fn.Entry()
//	bb.Return(x)
package vil
