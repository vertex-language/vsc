package gen

import (
	"github.com/vertex-language/vsc/analyzer"
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/internal/sil"
	"github.com/vertex-language/vsc/mangle"
	"github.com/vertex-language/vsc/stdlib"
	"github.com/vertex-language/vsc/token"
	"github.com/vertex-language/vsc/types"
)

// File lowers one checked file into a raw SIL module.
func File(name string, f *ast.File, info *analyzer.Info) (*sil.Module, []token.Diagnostic) {
	return Files(name, []*ast.File{f}, info)
}

// Files lowers all files in a module into a single raw SIL module.
func Files(name string, files []*ast.File, info *analyzer.Info) (*sil.Module, []token.Diagnostic) {
	m := sil.NewModule(name, sil.StageRaw)
	m.Import("Builtin")

	// Identify classes involved in inheritance hierarchies for vtable generation.
	poly := polymorphic(files, info)

	var diags []token.Diagnostic
	sawEntry := false
	// Identify script file (e.g. main.swift) containing top-level executable statements.
	script := scriptFile(name, files)
	var scriptStmts []ast.Stmt
	// Module-level variables first: a use in one file may come before the
	// declaration in another.
	vars := map[analyzer.Symbol]*moduleVar{}
	getters := map[analyzer.Symbol]*moduleGetter{}
	// A declaration's type is written once for the module: two files
	// calling the same function share its declaration.
	stated := map[*sil.Func]bool{}
	// A method of a generic type is lowered for each instance it is
	// called on, from wherever the call is: these are where they were
	// written.
	// The core's algorithms are read with the program's files: a method
	// of Array's the program calls is lowered from there, as one the
	// program's own extension declares is from here.
	lookup := files
	if info != nil && info.CoreAlgorithms != nil {
		lookup = append(append([]*ast.File{}, files...), info.CoreAlgorithms)
	}
	methods := genericMethodDecls(lookup, info)
	inits := genericInitDecls(lookup, info)
	publicTypes := map[string]bool{}
	for _, f := range files {
		if f == script {
			continue
		}
		pre := &gen{m: m, info: info, file: f.Unit, files: lookup, module: name, poly: poly, vars: vars, getters: getters, stated: stated, methods: methods, inits: inits, publicTypes: publicTypes}
		pre.declareModuleVars(f)
		for _, stmt := range f.Stmts {
			if decl, ok := stmt.(*ast.DeclStmt); ok {
				switch d := decl.D.(type) {
				case *ast.StructDecl:
					pre.publicMetadata(d.Name, d.Mods)
				case *ast.ClassDecl:
					pre.publicMetadata(d.Name, d.Mods)
				case *ast.EnumDecl:
					pre.publicMetadata(d.Name, d.Mods)
				}
			}
		}
		diags = append(diags, pre.diags...)
	}
	for _, f := range files {
		g := &gen{m: m, info: info, file: f.Unit, files: lookup, module: name, poly: poly, vars: vars, getters: getters, stated: stated, methods: methods, inits: inits, publicTypes: publicTypes, script: script != nil}
		reportedTopLevel := false
		for _, stmt := range f.Stmts {
			decl, ok := stmt.(*ast.DeclStmt)
			if !ok {
				// Top-level statements outside a script file (e.g. main.swift) are disallowed.
				if _, isConfig := stmt.(*ast.IfConfigStmt); isConfig {
					continue
				}
				if _, isEmpty := stmt.(*ast.EmptyStmt); isEmpty {
					continue
				}
				if f == script {
					scriptStmts = append(scriptStmts, stmt)
					continue
				}
				if !reportedTopLevel {
					reportedTopLevel = true
					g.errorAt(stmt, "top-level code runs only in main.swift: "+
						"put these statements in a function, or in main.swift")
					diags = append(diags, g.diags...)
					g.diags = nil
				}
				continue
			}
			// Top-level var in script file is lowered as a top-level statement.
			if _, isVar := decl.D.(*ast.VarDecl); isVar && f == script {
				scriptStmts = append(scriptStmts, stmt)
				continue
			}
			if fn, isFunc := decl.D.(*ast.FuncDecl); isFunc && fn.Recv == nil && script == nil &&
				fn.Name != nil && name == EntryModule && fn.Name.Text(f.Unit) == EntryName {
				sawEntry = true
			}
			switch d := decl.D.(type) {
			case *ast.FuncDecl:
				// Generic functions are monomorphized at call sites; skip top-level emission.
				if sym, ok := g.info.Defs[d.Name].(*analyzer.FuncSymbol); ok &&
					len(sym.Signature().TypeParams) > 0 {
					continue
				}
				// Lower receiver method attached to nominal type.
				g.function(d, g.info.Receivers[d])
			case *ast.StructDecl:
				g.publicMetadata(d.Name, d.Mods)
				g.members(d.Name, d.Body)
			case *ast.ClassDecl:
				g.publicMetadata(d.Name, d.Mods)
				g.members(d.Name, d.Body)
			case *ast.ActorDecl:
				g.members(d.Name, d.Body)
			case *ast.EnumDecl:
				g.publicMetadata(d.Name, d.Mods)
				g.members(d.Name, d.Body)
			case *ast.ExtensionDecl:
				// Lower methods declared inside extensions.
				g.extension(d)
			case *ast.VarDecl:
				g.emitModuleVars(d)
			}
		}
		diags = append(diags, g.diags...)
	}

	if script != nil {
		g := &gen{m: m, info: info, file: script.Unit, files: lookup, module: name, poly: poly, vars: vars, getters: getters, stated: stated, methods: methods, inits: inits, script: true}
		g.topLevelMain(scriptStmts)
		diags = append(diags, g.diags...)
		sawEntry = true
	}

	// Emit vtables and witness tables after all function symbols exist.
	tg := &gen{m: m, info: info, files: lookup, module: name, poly: poly, vars: vars, getters: getters, stated: stated, methods: methods, inits: inits, script: script != nil}
	if len(files) > 0 {
		tg.file = files[0].Unit
	}
	tg.vtables(files)
	tg.witnessTables(files)
	diags = append(diags, tg.diags...)

	// Validate entry point existence for main module.
	if name == EntryModule && !sawEntry && len(files) > 0 {
		diags = append(diags, token.Diagnostic{
			Pos:      files[0].Pos(),
			End:      files[0].Pos(),
			Severity: token.Error,
			File:     files[0].Unit,
			Message: "no entry point: module '" + EntryModule +
				"' has no 'func " + EntryName + "()'. A library is built with -module",
		})
	}
	return m, diags
}

// polymorphic returns all classes that have or serve as a superclass.
func polymorphic(files []*ast.File, info *analyzer.Info) map[*types.Class]bool {
	out := map[*types.Class]bool{}
	var mark func(t types.Type)
	mark = func(t types.Type) {
		if t == nil {
			return
		}
		if cl, ok := t.Underlying().(*types.Class); ok {
			out[cl] = true
		}
	}
	for _, f := range files {
		for _, stmt := range f.Stmts {
			decl, ok := stmt.(*ast.DeclStmt)
			if !ok {
				continue
			}
			cd, ok := decl.D.(*ast.ClassDecl)
			if !ok || cd.Name == nil {
				continue
			}
			sym, _ := info.Defs[cd.Name].(*analyzer.TypeNameSymbol)
			if sym == nil {
				continue
			}
			cl, ok := sym.Type().Underlying().(*types.Class)
			if !ok || cl.Superclass == nil {
				continue
			}
			// Mark both subclass and superclass as polymorphic.
			mark(cl)
			mark(cl.Superclass)
		}
	}
	return out
}

// extension lowers methods declared within an extension.
func (g *gen) extension(d *ast.ExtensionDecl) {
	recv := g.info.Extensions[d]
	if recv == nil || d.Body == nil {
		return
	}
	// A generic built-in type's are lowered where they are used, as a
	// generic type's are.
	if builtinParams(g.info, recv) != nil {
		return
	}
	if len(nominalTypeParams(recv)) > 0 {
		// A generic type's members are lowered where they are used.
		return
	}
	// A computed property an extension adds has a getter to emit, as one
	// the type's body declares does.
	g.emitGetters(d.Body, recv)
	g.emitSubscripts(d.Body, recv)
	for _, mem := range d.Body.Members {
		switch m := mem.(type) {
		case *ast.FuncDecl:
			g.function(m, recv)
		case *ast.InitDecl:
			g.initializer(m, recv)
		}
	}
}

func (g *gen) members(name *ast.Ident, body *ast.MemberBlock) {
	if name == nil || body == nil {
		return
	}
	sym, _ := g.info.Uses[name].(*analyzer.TypeNameSymbol)
	if sym == nil {
		if def, ok := g.info.Defs[name].(*analyzer.TypeNameSymbol); ok {
			sym = def
		}
	}
	if sym == nil {
		return
	}
	g.checkStoredExistentials(name, sym.Type())
	// A computed property is a function, and needs one emitted: the
	// call site names its getter's symbol, and nothing would define
	// it. See computed.go.
	// A generic type's are lowered where they are used.
	if len(nominalTypeParams(sym.Type())) == 0 {
		g.emitGetters(body, sym.Type())
		g.emitSubscripts(body, sym.Type())
	}
	g.emitStatics(sym.Type())
	for _, mem := range body.Members {
		switch m := mem.(type) {
		case *ast.FuncDecl:
			if len(nominalTypeParams(sym.Type())) > 0 {
				continue
			}
			g.function(m, sym.Type())
		case *ast.InitDecl:
			// A generic type's initializers are lowered where they are called.
			if len(nominalTypeParams(sym.Type())) > 0 {
				continue
			}
			g.initializer(m, sym.Type())
		case *ast.DeinitDecl:
			if len(nominalTypeParams(sym.Type())) > 0 {
				g.refuse(m, "a deinit of a generic class")
				continue
			}
			g.deinitializer(m, sym.Type())

		// Nested type members.
		case *ast.StructDecl:
			g.members(m.Name, m.Body)
		case *ast.ClassDecl:
			g.members(m.Name, m.Body)
		case *ast.ActorDecl:
			g.members(m.Name, m.Body)
		case *ast.EnumDecl:
			g.members(m.Name, m.Body)
		}
	}
}

// checkExistentialSignature checks whether a function signature contains unsupported existential returns (@out).
func (g *gen) checkExistentialSignature(d *ast.FuncDecl, sig *types.Signature) bool {
	if _, isEx := existentialOf(sig.Results); isEx {
		g.refuse(d, "a function whose result is an existential, which is returned by "+
			"filling in storage the caller set aside rather than in registers")
		return false
	}
	return true
}

// checkStoredExistentials refuses types with stored existential properties (address-only types).
func (g *gen) checkStoredExistentials(at ast.Node, t types.Type) {
	var fields []*types.Field
	switch b := t.Underlying().(type) {
	case *types.Struct:
		fields = b.Fields
	case *types.Class:
		fields = b.Fields
	default:
		return
	}
	for _, f := range fields {
		if f == nil {
			continue
		}
		if _, ok := existentialOf(f.Type); ok {
			g.refuse(at, "'"+f.Name+"', a stored property whose type is an "+
				"existential: a type holding one is held in memory rather than "+
				"in registers, and this builds every struct in registers")
			return
		}
	}
}

// symbol returns the mangled linkage symbol for a function.
func (g *gen) symbol(sym *analyzer.FuncSymbol) string {
	// The entry point is unmangled (main).
	if g.isEntry(sym) {
		return EntryName
	}
	// Honor @_silgen_name attribute if present.
	if name, has := g.silgenName(sym); has && name != "" {
		return name
	}
	// Nested local functions include an enclosing discriminator.
	if enc, ok := g.nested[sym]; ok {
		d := mangle.Decl{
			Module:        g.moduleOf(sym),
			Name:          sym.Name(),
			Signature:     sym.Signature(),
			Discriminator: mangle.Discriminator(g.file.Name() + "\x00" + enc),
			ModuleOf:      g.moduleOfType,
		}
		if name, err := mangle.Function(d); err == nil {
			return name
		}
	}
	d := mangle.Decl{
		Module:    g.moduleOf(sym),
		Name:      sym.Name(),
		Signature: sym.Signature(),
		ModuleOf:  g.moduleOfType,
	}
	// Private symbols include filename discriminator.
	switch sym.Access() {
	case analyzer.Private, analyzer.FilePrivate:
		d.Discriminator = mangle.Discriminator(g.file.Name())
	}
	name, err := mangle.Function(d)
	if err != nil {
		g.cannotName(sym, err)
		return sym.Name()
	}
	return name
}

// cannotName reports an error when symbol mangling fails for a declaration.
func (g *gen) cannotName(sym *analyzer.FuncSymbol, err error) {
	msg := "cannot name '" + sym.Name() + "': " + err.Error()
	if module, imported := g.info.Imported[sym]; imported {
		g.diags = append(g.diags, token.Diagnostic{
			Severity: token.Error,
			Message: "cannot name '" + sym.Name() + "', declared in module " +
				module + ": " + err.Error(),
		})
		return
	}
	g.diags = append(g.diags, token.Diagnostic{
		Pos:      sym.Pos(),
		End:      sym.Pos(),
		Severity: token.Error,
		Message:  msg,
		File:     g.file,
	})
}

// linkageOf maps an access level to its corresponding SIL linkage.
func linkageOf(a analyzer.Access) sil.Linkage {
	switch a {
	case analyzer.Public, analyzer.Open:
		return sil.Public
	case analyzer.Package:
		return sil.PackageLinkage
	case analyzer.Private, analyzer.FilePrivate:
		return sil.Private
	}
	return sil.Hidden
}

// A gen lowers one file.
type gen struct {
	m         *sil.Module
	info      *analyzer.Info
	file      *token.File
	files     []*ast.File              // every file of the module: code written in one is lowered in another
	nodeFiles map[ast.Node]*token.File // which file a node came from, as fileOf finds it
	module    string

	fn     *sil.Func
	entry  bool // fn is the program's entry point
	script bool // true if entry point is script file (main.swift) top-level statements
	blk    *sil.Block
	scopes []*scope
	locals map[analyzer.Symbol]*local

	loops   []loop
	pending string

	recv        types.Type                  // receiver type for method being lowered, or nil
	closures    int                         // count of emitted closures (for unique names)
	armOwned    map[*sil.Block][]*sil.Value // payloads a case arm owns, let go when its scope ends
	loopCase    *loopElement                // the element a `for case` body matches, before it runs
	poly        map[*types.Class]bool
	nested      map[analyzer.Symbol]string
	stated      map[*sil.Func]bool
	methods     map[genericMethodKey][]*ast.FuncDecl // generic types' methods, shared by every file
	props       map[genericMethodKey]genericProperty // generic types' computed properties, found when first needed
	inits       map[types.Type][]*ast.InitDecl       // generic types' initializers, shared by every file
	subst       map[*types.TypeParam]types.Type
	specialized map[string]bool
	tables      map[string]bool
	self        *local // receiver storage for mutating initializers
	initReturn  func() // early return handler for initializers

	throws  bool          // the function being lowered is declared throws
	catches []catchTarget // enclosing do/catch statements, innermost last
	tryBang bool          // the next call that may fail is under try!
	// tryCall is the method call a `try?` or `try!` is written on, and
	// what the keyword asks of it; the call takes them at its apply.
	tryCall     *ast.CallExpr
	vars        map[analyzer.Symbol]*moduleVar    // module-level variables, shared by every file
	getters     map[analyzer.Symbol]*moduleGetter // computed module-level variables, shared by every file
	staticRecv  types.Type                        // the type whose static initializer is being lowered
	localFunc   bool                              // lowering a function declared inside another
	tryOptional bool
	tryTrap     bool
	publicTypes map[string]bool // nominal types this module exports

	// storage is the addresses an expression answered that are a variable's
	// own storage rather than a temporary: an existential local, named. A
	// use that keeps the value copies out of one of these, and takes a
	// temporary over.
	storage map[*sil.Value]bool

	// chainNone is where an `a?` that finds nothing goes, for each optional
	// chain being lowered, innermost last; chainActive is the roots whose
	// steps are being lowered, which are typed as their unwrapped result.
	chainNone []*sil.Block
	// writebacks are what the current statement wrote through the
	// temporaries of declared subscripts, set back when it ends. See
	// subscript.go.
	writebacks  []func()
	chainDepth  []int // the scope each chain in chainNone opened, let go of on its way there
	chainActive map[ast.Expr]bool
	// lateReceivers are the calls whose receiver is evaluated after their
	// arguments: a mutating method on an array element. See elementAddr.
	lateReceivers map[*ast.CallExpr]bool
	// payloadTests collects, while a case's payload is bound, the parts
	// its pattern matches rather than binds. See enumItemArm.
	payloadTests *[]payloadTest

	diags []token.Diagnostic
}

// publicMetadata emits the runtime type metadata of a public type.
//
// A module compiled against this one assumes the metadata of every type
// it can name is a symbol this module exports -- see needMetadata, which
// emits nothing for a type from elsewhere. Emitting it only where this
// module happened to need it left a client that casts to a public type,
// or prints one, with an undefined accessor at the link.
//
// A generic type has no metadata of its own: each specialization is its
// own record, emitted where it is made.
func (g *gen) publicMetadata(name *ast.Ident, mods []*ast.Modifier) {
	if name == nil || g.accessLinkage(mods) != sil.Public {
		return
	}
	sym, _ := g.info.Defs[name].(*analyzer.TypeNameSymbol)
	if sym == nil || sym.Type() == nil {
		return
	}
	if len(nominalTypeParams(sym.Type())) > 0 || len(nominalChain(sym.Type())) == 0 {
		return
	}
	if g.publicTypes == nil {
		g.publicTypes = map[string]bool{}
	}
	g.publicTypes[typeNameOf(sym.Type())] = true
	// Quietly: a type this cannot describe is one a client cannot name at
	// run time either, and that is an error at the use that needs it, not
	// at a declaration this module is otherwise happy to compile.
	before := len(g.diags)
	if !g.needMetadata(name, sym.Type()) {
		g.diags = g.diags[:before]
	}
}

// unsupported records an error for an unhandled expression.
func (g *gen) unsupported(e ast.Expr) {
	name := "an expression"
	if e != nil {
		name = g.exprKind(e)
	}
	g.refuse(e, name)
}

// unsupportedStmt records an error for an unhandled statement.
func (g *gen) unsupportedStmt(s ast.Stmt) {
	name := "a statement"
	if s != nil {
		name = stmtKind(s)
	}
	g.refuse(s, name)
}

// errorAt records a diagnostic error anchored to an AST node.
func (g *gen) errorAt(n ast.Node, msg string) {
	pos, end := token.NoPos, token.NoPos
	if n != nil {
		pos, end = n.Pos(), n.End()
	}
	g.diags = append(g.diags, token.Diagnostic{
		Pos:      pos,
		End:      end,
		Severity: token.Error,
		Message:  msg,
		File:     g.file,
	})
}

// refuse records a "cannot lower yet" diagnostic for an unhandled AST node.
func (g *gen) refuse(n ast.Node, name string) {
	pos, end := token.NoPos, token.NoPos
	if n != nil {
		pos, end = n.Pos(), n.End()
	}
	g.diags = append(g.diags, token.Diagnostic{
		Pos:      pos,
		End:      end,
		Severity: token.Error,
		Message:  "cannot lower " + name + " yet",
		File:     g.file,
	})
}

// stmtKind names a statement the way a person would say it.
func stmtKind(s ast.Stmt) string {
	switch s.(type) {
	case *ast.ForInStmt:
		return "a for-in loop"
	case *ast.WhileStmt:
		return "a while loop"
	case *ast.RepeatWhileStmt:
		return "a repeat-while loop"
	case *ast.SwitchStmt:
		return "a switch"
	case *ast.GuardStmt:
		return "a guard"
	case *ast.DeferStmt:
		return "a defer"
	case *ast.DoStmt:
		return "a do block"
	case *ast.ThrowStmt:
		return "a throw"
	case *ast.BreakStmt:
		return "a break"
	case *ast.ContinueStmt:
		return "a continue"
	case *ast.FallthroughStmt:
		return "a fallthrough"
	case *ast.LabeledStmt:
		return "a labelled statement"
	case *ast.YieldStmt:
		return "a yield"
	case *ast.DiscardStmt:
		return "a discard"
	case *ast.IfConfigStmt:
		return "a compiler directive"
	}
	return "this statement"
}

func (g *gen) exprKind(e ast.Expr) string {
	switch n := e.(type) {
	case *ast.PrefixExpr:
		return "this prefix operator"
	case *ast.PostfixExpr:
		return "a postfix operator"
	case *ast.ClosureExpr:
		return "a closure"
	case *ast.SubscriptExpr:
		return "a subscript"
	case *ast.TernaryExpr, *ast.ConditionalExpr:
		return "a conditional expression"
	case *ast.ArrayLit:
		return "an array literal"
	case *ast.DictLit:
		return "a dictionary literal"
	case *ast.TupleExpr:
		return "a tuple expression"
	case *ast.CallExpr:
		if id, ok := n.Fun.(*ast.IdentExpr); ok && id.Name != nil {
			// A name that is neither a function nor a value of
			// function type is a type, and calling one makes an
			// instance. Saying which reads very differently to
			// whoever wrote the call, so the two are told apart
			// rather than both called "this call".
			if _, isFunc := g.info.Uses[id.Name].(*analyzer.FuncSymbol); !isFunc {
				if _, callable := g.typeOf(id).Underlying().(*types.Signature); !callable {
					return "a constructor call"
				}
			}
		}
		return "this call"
	}
	return "this expression"
}

// moduleOf is the module a symbol belongs to, which is the one being
// compiled unless the symbol came from an interface.
//
// A symbol's mangled name carries its module, and that is what makes
// two modules' symbols distinct -- so a call to an imported function
// has to mangle with the module that will define it rather than with
// the one making the call. Getting this wrong does not fail at
// compile time: it fails at link time, on a symbol nobody defined,
// with the caller's module name in it.
func (g *gen) moduleOf(sym analyzer.Symbol) string {
	if g.info != nil {
		if m, ok := g.info.Imported[sym]; ok && m != "" {
			return m
		}
	}
	return g.module
}

// moduleOfType is the module a nominal type was declared in, which is
// what a method of it is mangled with.
func (g *gen) moduleOfType(t types.Type) string {
	if g.info == nil || t == nil {
		return g.module
	}
	// Error is the universe's and the others core's: all the standard
	// library's, as Swift spells them.
	if p, ok := t.(*types.Protocol); ok && (p == types.ErrorProtocol || g.isCoreType(p)) {
		return "Swift"
	}
	if m, ok := g.info.ImportedTypes[t]; ok && m != "" {
		return m
	}
	if m, ok := g.info.ImportedTypes[t.Underlying()]; ok && m != "" {
		return m
	}
	// A type core declares belongs to the standard library, whichever
	// module is being compiled. Without this every module mangled
	// ArraySlice as its own, and a library and its client disagreed about
	// the name of a function taking one.
	if g.isCoreType(t) {
		return "Swift"
	}
	// A type declared inside another is in whatever module that one
	// is in. Nothing records it separately: what the checker recorded
	// when it read an interface is the names that interface declared
	// at its top level, and `Chart.Point` is not one of them.
	if in := enclosingType(t); in != nil {
		return g.moduleOfType(in)
	}
	return g.module
}

// isCoreType reports whether core declares t: the type itself, or -- for
// `ArraySlice<UInt8>` -- the generic type it is an instance of.
func (g *gen) isCoreType(t types.Type) bool {
	if g.info == nil || t == nil {
		return false
	}
	if g.info.CoreTypes[t] || g.info.CoreTypes[t.Underlying()] {
		return true
	}
	if inst, ok := t.(*types.GenericInstance); ok && inst.Base != nil {
		return g.info.CoreTypes[inst.Base] || g.info.CoreTypes[inst.Base.Underlying()]
	}
	return false
}

// enclosingType is the type a nominal one is declared inside, or nil.
func enclosingType(t types.Type) types.Type {
	switch n := t.(type) {
	case *types.Struct:
		return n.In
	case *types.Class:
		return n.In
	case *types.Enum:
		return n.In
	case *types.GenericInstance:
		return enclosingType(n.Base)
	}
	return nil
}

// local represents a scoped binding (a let value, or a var stack/box address).
type local struct {
	value *sil.Value // let: value
	addr  *sil.Value // var: storage address
	box   *sil.Value // var: box (if boxed)
	typ   sil.Type
	mem   bool // true for memory-only storage (e.g. existentials)
}

// scope collects cleanups (destroys, end_borrow, end_access) to emit on exit.
// formal scopes end with individual statements; lexical scopes end with blocks.
type scope struct {
	cleanups []cleanup
	formal   bool
}

// cleanup represents a deferred destroy, end_borrow, or end_access operation.
type cleanup struct {
	destroy *sil.Value
	// destroyAddr is storage whose contents are destroyed in place:
	// an existential a catch copied out of its box.
	destroyAddr *sil.Value
	endBorrow   *sil.Value
	endAccess   *sil.Value

	// freeCString is the copy a String passed as a C string was made into,
	// let go of once the call it was made for is over. See cStringArg.
	freeCString *sil.Value

	// deferred is a `defer` block: lowered again on every path that leaves
	// the scope it was written in -- falling off the end, return, break,
	// continue, and throw -- after whatever was registered later.
	deferred *ast.CodeBlock
}

func (g *gen) push()       { g.scopes = append(g.scopes, &scope{}) }
func (g *gen) pushFormal() { g.scopes = append(g.scopes, &scope{formal: true}) }
func (g *gen) top() *scope { return g.scopes[len(g.scopes)-1] }

// lexical returns the innermost non-formal (block) scope.
func (g *gen) lexical() *scope {
	for i := len(g.scopes) - 1; i >= 0; i-- {
		if !g.scopes[i].formal {
			return g.scopes[i]
		}
	}
	return g.scopes[0]
}

// destroyLater registers an owned value for cleanup at the end of the current lexical scope.
func (g *gen) destroyLater(v *sil.Value) {
	if v == nil || v.Ownership() != sil.Owned {
		return
	}
	if g.pendingDestroy(v) {
		return
	}
	s := g.lexical()
	s.cleanups = append(s.cleanups, cleanup{destroy: v})
}

// destroyTemp registers an owned temporary -- a copy made to read a
// variable -- for cleanup at the end of the statement it was made in,
// which is where Swift ends a temporary: the copy exists for the
// expression, and once the statement is over the variable is the only
// holder again. Registered at the block's end instead, the copy would
// outlive the statement, and a write to the variable in a later statement
// would find its storage shared and copy all of it.
func (g *gen) destroyTemp(v *sil.Value) {
	if v == nil || v.Ownership() != sil.Owned {
		return
	}
	if g.pendingDestroy(v) {
		return
	}
	s := g.top()
	if !s.formal {
		s = g.lexical()
	}
	s.cleanups = append(s.cleanups, cleanup{destroy: v})
}

// destroyAddrLater registers storage whose value is destroyed in place at
// the end of the current lexical scope.
func (g *gen) destroyAddrLater(addr *sil.Value) {
	if addr == nil {
		return
	}
	s := g.lexical()
	s.cleanups = append(s.cleanups, cleanup{destroyAddr: addr})
}

// endBorrowLater registers a borrow scope cleanup at the end of the formal scope.
func (g *gen) endBorrowLater(v *sil.Value) {
	s := g.top()
	s.cleanups = append(s.cleanups, cleanup{endBorrow: v})
}

// endAccessLater registers an access scope cleanup at the end of the formal scope.
func (g *gen) endAccessLater(v *sil.Value) {
	s := g.top()
	s.cleanups = append(s.cleanups, cleanup{endAccess: v})
}

// forget cancels a pending cleanup when ownership is transferred (e.g. returned or stored).
func (g *gen) forget(v *sil.Value) {
	if v == nil {
		return
	}
	for _, s := range g.scopes {
		for i, c := range s.cleanups {
			if c.destroy == v {
				s.cleanups = append(s.cleanups[:i], s.cleanups[i+1:]...)
				return
			}
		}
	}
}

// pop emits the current scope's cleanups and leaves it.
func (g *gen) pop() {
	g.emitCleanups(g.top())
	g.scopes = g.scopes[:len(g.scopes)-1]
}

// popReachable emits cleanups if the current block is unterminated, then pops the scope.
func (g *gen) popReachable() {
	if g.blk != nil && g.blk.Term() == nil {
		g.pop()
		return
	}
	g.scopes = g.scopes[:len(g.scopes)-1]
}

// unwind emits cleanups for all open scopes without popping them.
func (g *gen) unwind() { g.unwindTo(0) }

// unwindTo emits cleanups down to the specified scope depth.
func (g *gen) unwindTo(depth int) {
	for i := len(g.scopes) - 1; i >= depth; i-- {
		g.emitCleanups(g.scopes[i])
	}
}

// loop tracks control flow targets for break and continue.
type loop struct {
	header *sil.Block // continue: test the condition again
	exit   *sil.Block // break: the statement after the loop
	depth  int
	label  string // "" unless the loop was written with one

	lazyExit func() *sil.Block // Creates exit block only when branched to

	// isSwitch marks the entry a switch pushes so that `break` leaves
	// it: `continue` looks past it to the loop it is in, whose depth is
	// where the unwinding has to reach.
	isSwitch bool
}

// exitBlock returns or lazily instantiates the loop exit block.
func (l loop) exitBlock() *sil.Block {
	if l.exit != nil {
		return l.exit
	}
	if l.lazyExit != nil {
		return l.lazyExit()
	}
	return nil
}

// enclosing finds the target loop matching the given label, or innermost if label is empty.
func (g *gen) enclosing(label string) (loop, bool) {
	return g.enclosingFor(label, false)
}

// enclosingFor is enclosing, looking past the entries switches push
// when continuing: a `continue` in a switch's arm goes to the loop
// around the switch, and unwinds everything between.
func (g *gen) enclosingFor(label string, continuing bool) (loop, bool) {
	for i := len(g.loops) - 1; i >= 0; i-- {
		if continuing && g.loops[i].isSwitch {
			continue
		}
		if label == "" || g.loops[i].label == label {
			return g.loops[i], true
		}
	}
	return loop{}, false
}

// deferBody lowers a `defer` block where control leaves its scope. The
// block cannot break or continue out of itself, so the loops around the
// defer are not its targets.
func (g *gen) deferBody(b *ast.CodeBlock) {
	loops := g.loops
	g.loops = nil
	g.block(b)
	g.loops = loops
}

// emitCleanups emits destruction and borrow-ending instructions in reverse registration order.
func (g *gen) emitCleanups(s *scope) {
	for i := len(s.cleanups) - 1; i >= 0; i-- {
		if g.blk == nil || g.blk.Term() != nil {
			return // a deferred block that did not come back ends the path
		}
		switch c := s.cleanups[i]; {
		case c.deferred != nil:
			g.deferBody(c.deferred)
		case c.freeCString != nil:
			g.runtimeResult(stdlib.StringCStringFree,
				[]sil.Param{{Type: rawPointerType(), Convention: sil.ParamUnowned}},
				lowerType(types.Typ[types.Void]), c.freeCString)
		case c.endBorrow != nil:
			g.blk.EndBorrow(c.endBorrow)
		case c.endAccess != nil:
			g.blk.EndAccess(c.endAccess)
		case c.destroyAddr != nil:
			g.blk.DestroyAddr(c.destroyAddr)
		case c.destroy != nil:
			// Existential containers require destroy_addr rather than destroy_value.
			if isExistentialType(c.destroy.Type().Formal()) {
				g.blk.DestroyAddr(c.destroy)
				continue
			}
			g.blk.DestroyValue(c.destroy)
		}
	}
}

// function lowers one declaration.
func (g *gen) function(d *ast.FuncDecl, recv types.Type) {
	g.functionNamed(d, recv, "")
}

// functionNamed lowers a function declaration with an explicit mangled symbol name.
func (g *gen) functionNamed(d *ast.FuncDecl, recv types.Type, symbol string) {
	sym, _ := g.info.Defs[d.Name].(*analyzer.FuncSymbol)
	if sym == nil {
		return
	}
	sig, _ := g.substituted(sym.Signature()).(*types.Signature)
	if sig == nil {
		sig = sym.Signature()
	}
	g.recv = recv

	// Reject reserved execution modifiers lacking backend support.
	if d.Sig != nil && d.Sig.Exec != ast.ExecNone {
		g.errorAt(d.Name, "cannot lower a "+d.Sig.Exec.String()+
			" function yet: the modifier is reserved and has no backend")
		return
	}

	if !g.checkExistentialSignature(d, sig) {
		return
	}

	if !g.interopAttrs(d) {
		return
	}

	// Validate entry point signature if this function is the main entry.
	g.entry = g.isEntry(sym)
	if g.entry && !g.checkEntry(sym, sig) {
		g.entry = false
		return
	}
	// An async main is an ordinary function run as the first task; the
	// program's entry is a function of its own that starts it.
	asyncEntry := g.entry && sig.Async
	// A throwing main is an ordinary throwing function; the entry calls
	// it, and an error it throws ends the program. One that is both is
	// wrapped in an async function that catches, run as an async main.
	throwingEntry := g.entry && sig.Throws && !sig.Async
	if asyncEntry || throwingEntry {
		g.entry = false
	}

	name := symbol
	if name == "" {
		name = g.symbol(sym)
	}
	entryName := name
	if asyncEntry {
		name = asyncEntryName
	}
	if throwingEntry {
		name = throwingEntryName
	}
	static := isStaticDecl(d.Mods)
	if recv != nil && symbol == "" {
		name = g.methodSymbol(&analyzer.MethodRef{
			Recv:   recv,
			Method: &types.Method{Name: sym.Name(), Sig: sig, IsStatic: static},
		})
	}

	// External declarations (@_silgen_name) have no body; emit declaration signature only.
	if d.Body == nil {
		if silgen, has := g.silgenName(sym); has && silgen != "" {
			f := g.m.Func(name).SetSourceName(sym.Name())
			if g.needsType(f) {
				g.declare(f, sym)
			}
			return
		}
		// Reject functions lacking both a body and an external symbol name.
		g.errorAt(d, "'"+sym.Name()+"' has no body: a function without one "+
			"needs '@_silgen_name(\"…\")' to say which symbol it is")
		return
	}

	linkage := linkageOf(sym.Access())
	if g.entry {
		linkage = sil.Public
	}
	// Reject symbol collisions.
	if existing := g.m.Lookup(name); existing != nil && !existing.IsDeclaration() {
		g.errorAt(d, "'"+sym.Name()+"' and something else in this module are "+
			"both '"+name+"'")
		return
	}
	f := g.m.Func(name).SetSourceName(sym.Name()).
		SetLinkage(linkage).SetAttr("ossa")
	g.fn = f
	// Pre-register all nested functions in this body.
	g.registerNested(d.Body, name)
	// Reset forward-declared parameters to overwrite with canonical definition types.
	f.Type().Params = nil
	g.locals = map[analyzer.Symbol]*local{}
	g.scopes = nil
	g.loops, g.pending = nil, ""
	g.throws, g.catches, g.tryBang = sig.Throws, nil, false
	if sig.Throws {
		f.SetThrows(sil.Object(sil.BuiltinNativeObj))
	}
	// An async function's type says so, as it does in Swift: `@async` is
	// part of the type and not a note on the declaration, because the
	// calling convention differs. See docs/vertex_swift_async.md.
	f.Type().Async = sig.Async
	f.Type().Isolated = sig.Isolated
	// Reset per-function self storage pointer.
	g.self = nil
	g.push()
	g.blk = f.Entry()

	// Parameters in declaration order.
	params := paramSymbols(d, g.info, g.file)
	for i, p := range sig.Params {
		t := lowerType(p.BodyType())
		conv := paramConvention(p, t)
		if byAddress(conv) {
			v := f.Param(t.Address(), conv)
			if i < len(params) && params[i] != nil {
				// Inout parameters and existentials are handled by address.
				_, isEx := existentialOf(p.Type)
				g.locals[params[i]] = &local{addr: v, typ: t, mem: isEx}
			}
			if p.Name != "" {
				g.blk.DebugValue(v, p.Name, "let", "argno "+itoa(i+1))
			}
			continue
		}
		v := f.Param(t, conv)
		if i < len(params) && params[i] != nil {
			g.locals[params[i]] = &local{value: v, typ: t}
		}
		if name := p.Name; name != "" {
			g.blk.DebugValue(v, name, "let", "argno "+itoa(i+1))
		}
		// Callee cleans up @owned parameters at scope exit.
		g.destroyLater(v)
	}
	if recv != nil && !static {
		// Append self as trailing parameter. Mutating value-type methods take self as @inout address.
		t := lowerType(recv)
		mutates := g.mutatingSelf(d, recv)
		var self *sil.Value
		if mutates {
			self = f.Param(t.Address(), sil.ParamInout)
			g.self = &local{addr: self, typ: t}
		} else {
			self = f.Param(t, selfConvention(t))
		}
		f.Type().Convention = sil.Method
		// Bind receiver symbol if named in declaration.
		if sym := receiverSymbol(d, g.info, g.file); sym != nil {
			if mutates {
				g.locals[sym] = &local{addr: self, typ: t}
			} else {
				g.locals[sym] = &local{value: self, typ: t}
			}
			g.blk.DebugValue(self, d.Recv.Name.Text(g.file), "let",
				"argno "+itoa(len(sig.Params)+1))
		}
	}

	switch {
	case g.entry:
		entrySignature(f)
	case sig.Results != nil && !isVoid(sig.Results):
		f.SetResult(lowerType(sig.Results), resultConvention(lowerType(sig.Results)))
	}

	g.prologueHop(d.Body.Stmts)
	g.block(d.Body)
	// Handle fallthrough at end of function (unreachable or void return).
	if g.blk != nil && g.blk.Term() == nil {
		g.unwind()
		if !g.entry && sig.Results != nil && !isVoid(sig.Results) {
			kind := "global function"
			switch {
			case g.localFunc:
				kind = "local function"
			case recv != nil && static:
				kind = "static method"
			case recv != nil:
				kind = "instance method"
			}
			g.missingReturn(d.Body.Lbrace, d.Body.Rbrace, kind, sig.Results)
		} else {
			g.blk.Return(g.result())
		}
	}
	g.fn = nil
	g.entry = false
	g.recv = nil
	if asyncEntry {
		if sig.Throws {
			f = g.asyncThrowingEntry(f)
		}
		g.asyncEntry(entryName, f)
	}
	if throwingEntry {
		g.throwingEntry(entryName, f)
	}

	// C entry point @_cdecl thunk if requested.
	g.cdeclThunk(d, sym, name)
}

// void is the empty tuple every function without a result returns.
func (g *gen) void() *sil.Value {
	return g.blk.Tuple(sil.Object(types.Typ[types.Void]))
}

func isVoid(t types.Type) bool {
	b, ok := t.(*types.Basic)
	return ok && b.Kind() == types.Void
}

// mutatingSelf reports whether this method mutates its receiver (@inout).
func (g *gen) mutatingSelf(d *ast.FuncDecl, recv types.Type) bool {
	if recv == nil || isClass(recv) {
		return false
	}
	if d.Recv != nil {
		for _, m := range d.Recv.Mods {
			if m != nil && m.Kind == token.INOUT {
				return true
			}
		}
	}
	for _, m := range d.Mods {
		if m != nil && m.Name != nil && g.text(m.Name) == "mutating" {
			return true
		}
	}
	return false
}

// receiverSymbol is the symbol a receiver method's receiver name was
// declared under, or nil where the function has no receiver clause.
func receiverSymbol(d *ast.FuncDecl, info *analyzer.Info, f *token.File) analyzer.Symbol {
	if d.Recv == nil || d.Recv.Name == nil {
		return nil
	}
	scope := info.Scopes[d]
	if scope == nil {
		return nil
	}
	return scope.Lookup(d.Recv.Name.Text(f))
}

func paramSymbols(d *ast.FuncDecl, info *analyzer.Info, f *token.File) []analyzer.Symbol {
	if d.Sig == nil {
		return nil
	}
	out := make([]analyzer.Symbol, 0, len(d.Sig.Params))
	for range d.Sig.Params {
		out = append(out, nil)
	}
	// Map parameter symbols from analyzer scope in declaration order.
	if scope := info.Scopes[d]; scope != nil {
		for i, p := range d.Sig.Params {
			name := p.Name
			if name == nil {
				name = p.Label
			}
			if name == nil {
				continue
			}
			if sym := scope.Lookup(name.Text(f)); sym != nil {
				out[i] = sym
			}
		}
	}
	return out
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// needsType reports whether a declaration's type still has to be
// written down, and records that it now has been.
func (g *gen) needsType(f *sil.Func) bool {
	if f == nil || !f.IsDeclaration() || f.Type() == nil {
		return false
	}
	if g.stated[f] {
		return false
	}
	if g.stated == nil {
		g.stated = map[*sil.Func]bool{}
	}
	g.stated[f] = true
	return true
}

// typeOf returns the type of expression e, applying generic substitutions if active.
func (g *gen) typeOf(e ast.Expr) types.Type {
	t := g.info.Types[e]
	if g.chainActive[e] {
		t = g.info.ChainInner[e]
	}
	if len(g.subst) == 0 || t == nil {
		return t
	}
	return types.Substitute(t, g.subst)
}

// substituted is a type with the specialization in force applied.
func (g *gen) substituted(t types.Type) types.Type {
	if len(g.subst) == 0 || t == nil {
		return t
	}
	return types.Substitute(t, g.subst)
}
