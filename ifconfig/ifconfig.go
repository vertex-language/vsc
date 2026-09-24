// Package ifconfig resolves conditional compilation: each `#if` block in
// a parsed file is replaced, where it stands, by the statements of the
// clause whose condition holds for the target being built -- or by
// nothing, where none does. What follows parsing then never sees an
// `#if`, and a declaration made in two clauses is made once.
package ifconfig

import (
	"strconv"
	"strings"

	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/token"
)

// Config is what the conditions are asked of.
type Config struct {
	// OS and Arch are the target's, as Swift spells them: "macOS",
	// "Linux", "Windows", "Android", "iOS"; "arm64", "x86_64".
	OS, Arch string
	// Flags are the compilation conditions defined, `-D NAME`.
	Flags map[string]bool
	// CanImport answers canImport(M); nil answers true for Swift alone.
	CanImport func(module string) bool
}

// Language is the Swift version the conditions compare against.
var Language = []int{6, 0}

// ForTarget is the configuration for a target named as ir names one --
// "aarch64/macos", "x86_64/windows".
func ForTarget(target string) Config {
	arch, os, found := strings.Cut(target, "/")
	if !found {
		arch, os, _ = strings.Cut(target, "-")
	}
	cfg := Config{Flags: map[string]bool{}}
	switch arch {
	case "aarch64", "arm64":
		cfg.Arch = "arm64"
	case "x86_64", "amd64":
		cfg.Arch = "x86_64"
	case "i386", "x86":
		cfg.Arch = "i386"
	default:
		cfg.Arch = arch
	}
	switch {
	case strings.HasPrefix(os, "macos"), strings.HasPrefix(os, "darwin"):
		cfg.OS = "macOS"
	case strings.HasPrefix(os, "ios"):
		cfg.OS = "iOS"
	case strings.HasPrefix(os, "windows"):
		cfg.OS = "Windows"
	case strings.HasPrefix(os, "android"):
		cfg.OS = "Android"
	case strings.HasPrefix(os, "linux"):
		cfg.OS = "Linux"
	default:
		cfg.OS = os
	}
	return cfg
}

// Resolve replaces every `#if` in f by its active clause's statements.
func Resolve(f *ast.File, cfg Config) {
	if f == nil || f.Unit == nil {
		return
	}
	r := &resolver{cfg: cfg, file: f.Unit}
	f.Stmts = r.stmts(f.Stmts)
	ast.Inspect(f, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.CodeBlock:
			x.Stmts = r.stmts(x.Stmts)
		case *ast.ClosureExpr:
			x.Stmts = r.stmts(x.Stmts)
		case *ast.CaseClause:
			x.Stmts = r.stmts(x.Stmts)
		case *ast.SwitchStmt:
			x.Cases = r.stmts(x.Cases)
		case *ast.MemberBlock:
			x.Members = r.members(x.Members)
		}
		return true
	})
}

type resolver struct {
	cfg  Config
	file *token.File
}

// active is the clause of an #if whose condition holds, or nil.
func (r *resolver) active(s *ast.IfConfigStmt) *ast.IfConfigClause {
	for _, cl := range s.Clauses {
		if cl == nil {
			continue
		}
		if cl.Cond == nil || r.holds(cl.Cond) {
			return cl
		}
	}
	return nil
}

func (r *resolver) stmts(in []ast.Stmt) []ast.Stmt {
	has := false
	for _, s := range in {
		if _, ok := s.(*ast.IfConfigStmt); ok {
			has = true
		}
	}
	if !has {
		return in
	}
	out := make([]ast.Stmt, 0, len(in))
	for _, s := range in {
		ic, ok := s.(*ast.IfConfigStmt)
		if !ok {
			out = append(out, s)
			continue
		}
		if cl := r.active(ic); cl != nil {
			out = append(out, r.stmts(cl.Stmts)...)
		}
	}
	return out
}

func (r *resolver) members(in []ast.Node) []ast.Node {
	has := false
	for _, m := range in {
		if _, ok := m.(*ast.IfConfigStmt); ok {
			has = true
		}
	}
	if !has {
		return in
	}
	out := make([]ast.Node, 0, len(in))
	for _, m := range in {
		ic, ok := m.(*ast.IfConfigStmt)
		if !ok {
			out = append(out, m)
			continue
		}
		cl := r.active(ic)
		if cl == nil {
			continue
		}
		for _, s := range r.stmts(cl.Stmts) {
			// A member clause holds declarations, each a statement.
			if d, ok := s.(*ast.DeclStmt); ok {
				out = append(out, d.D)
				continue
			}
			out = append(out, s)
		}
	}
	return out
}

func (r *resolver) text(id *ast.Ident) string {
	if id == nil {
		return ""
	}
	return id.Text(r.file)
}

// holds evaluates a condition.
func (r *resolver) holds(e ast.Expr) bool {
	switch x := e.(type) {
	case *ast.ParenExpr:
		return r.holds(x.X)
	case *ast.BasicLit:
		return string(r.file.Slice(x.Lo, x.Hi)) == "true"
	case *ast.IdentExpr:
		return r.cfg.Flags[r.text(x.Name)]
	case *ast.PrefixExpr:
		if x.Op != nil && string(r.file.Slice(x.Op.Pos(), x.Op.End())) == "!" {
			return !r.holds(x.X)
		}
	case *ast.BinaryExpr:
		if x.Op == nil {
			return false
		}
		switch string(r.file.Slice(x.Op.Pos(), x.Op.End())) {
		case "&&":
			return r.holds(x.X) && r.holds(x.Y)
		case "||":
			return r.holds(x.X) || r.holds(x.Y)
		}
	case *ast.PlatformCond:
		return r.platform(x)
	case *ast.SequenceExpr:
		// Unfolded: `a || b && c`, && binding tighter, as Swift has it.
		// Each || separates terms that each must all hold.
		any, all, term := false, true, false
		for _, el := range x.Elements {
			if op, ok := el.(*ast.OperatorExpr); ok {
				switch string(r.file.Slice(op.Pos(), op.End())) {
				case "||":
					any = any || (term && all)
					all, term = true, false
				}
				continue
			}
			all = all && r.holds(el)
			term = true
		}
		return any || (term && all)
	}
	return false
}

// platform evaluates os(), arch(), swift(), compiler(), canImport() and
// the rest.
func (r *resolver) platform(p *ast.PlatformCond) bool {
	arg := r.text(p.Arg)
	switch r.text(p.Name) {
	case "os":
		switch arg {
		case "OSX":
			return r.cfg.OS == "macOS"
		}
		return arg == r.cfg.OS
	case "arch":
		return arg == r.cfg.Arch
	case "swift", "compiler":
		return r.version(p)
	case "canImport":
		var parts []string
		for _, id := range p.Path {
			parts = append(parts, r.text(id))
		}
		module := strings.Join(parts, ".")
		if module == "" {
			module = arg
		}
		if r.cfg.CanImport != nil {
			return r.cfg.CanImport(module)
		}
		return module == "Swift"
	case "targetEnvironment":
		return false
	case "_endian":
		return arg == "little"
	case "_pointerBitWidth":
		return arg == "_64"
	case "_runtime":
		return arg == "_Native"
	case "hasFeature", "hasAttribute":
		return false
	}
	return false
}

// version compares the language version with swift(>=x) or swift(<x).
func (r *resolver) version(p *ast.PlatformCond) bool {
	var text string
	switch {
	case p.Ver != nil:
		text = string(r.file.Slice(p.Ver.Pos(), p.Ver.End()))
	case p.VerStr != nil:
		text = strings.Trim(string(r.file.Slice(p.VerStr.Pos(), p.VerStr.End())), "\"")
	default:
		return false
	}
	var want []int
	for _, part := range strings.Split(text, ".") {
		n, err := strconv.Atoi(part)
		if err != nil {
			return false
		}
		want = append(want, n)
	}
	cmp := 0
	for i := 0; i < len(want) || i < len(Language); i++ {
		a, b := 0, 0
		if i < len(Language) {
			a = Language[i]
		}
		if i < len(want) {
			b = want[i]
		}
		if a != b {
			if a < b {
				cmp = -1
			} else {
				cmp = 1
			}
			break
		}
	}
	op := string(r.file.Slice(p.Op, p.Op+2))
	if strings.HasPrefix(op, "<") {
		return cmp < 0
	}
	return cmp >= 0
}
