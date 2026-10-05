package gen

import (
	"github.com/vertex-language/vsc/analyzer"
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/internal/sil"
	"github.com/vertex-language/vsc/token"
	"github.com/vertex-language/vsc/types"
)

// bodyState is everything about the function being lowered, as opposed to
// what the module shares: the function, where lowering is in it, and what
// is pending until its scopes end.
type bodyState struct {
	file          *token.File
	specializing  bool
	fn            *sil.Func
	entry         bool
	blk           *sil.Block
	scopes        []*scope
	locals        map[analyzer.Symbol]*local
	loops         []loop
	pending       string
	recv          types.Type
	armOwned      map[*sil.Block][]*sil.Value
	loopCase      *loopElement
	subst         map[*types.TypeParam]types.Type
	self          *local
	initReturn    func()
	initFail      func()
	throws        bool
	catches       []catchTarget
	tryBang       bool
	tryCall       *ast.CallExpr
	staticRecv    types.Type
	localFunc     bool
	tryOptional   bool
	tryTrap       bool
	storage       map[*sil.Value]bool
	chainNone     []*sil.Block
	writebacks    []func()
	chainDepth    []int
	chainActive   map[ast.Expr]bool
	lateReceivers map[*ast.CallExpr]bool
	payloadTests  *[]payloadTest
}

// apart sets the function being lowered aside so that another can be
// lowered from the middle of it -- a core member emitted where it is first
// used -- and returns what puts it back. The other starts from nothing, as
// a function lowered at the top of the module does.
func (g *gen) apart() (restore func()) {
	saved := bodyState{
		g.file, g.specializing, g.fn, g.entry, g.blk, g.scopes, g.locals, g.loops, g.pending,
		g.recv, g.armOwned, g.loopCase, g.subst, g.self, g.initReturn, g.initFail,
		g.throws, g.catches, g.tryBang, g.tryCall, g.staticRecv, g.localFunc,
		g.tryOptional, g.tryTrap, g.storage, g.chainNone, g.writebacks, g.chainDepth,
		g.chainActive, g.lateReceivers, g.payloadTests,
	}
	g.put(bodyState{file: g.file})
	return func() { g.put(saved) }
}

// put makes s the state of the function being lowered.
func (g *gen) put(s bodyState) {
	g.file, g.specializing, g.fn, g.entry, g.blk = s.file, s.specializing, s.fn, s.entry, s.blk
	g.scopes, g.locals, g.loops, g.pending = s.scopes, s.locals, s.loops, s.pending
	g.recv, g.armOwned, g.loopCase, g.subst = s.recv, s.armOwned, s.loopCase, s.subst
	g.self, g.initReturn, g.initFail = s.self, s.initReturn, s.initFail
	g.throws, g.catches, g.tryBang, g.tryCall = s.throws, s.catches, s.tryBang, s.tryCall
	g.staticRecv, g.localFunc, g.tryOptional, g.tryTrap = s.staticRecv, s.localFunc, s.tryOptional, s.tryTrap
	g.storage, g.chainNone, g.writebacks, g.chainDepth = s.storage, s.chainNone, s.writebacks, s.chainDepth
	g.chainActive, g.lateReceivers, g.payloadTests = s.chainActive, s.lateReceivers, s.payloadTests
}
