package doc

import (
	"regexp"
	"strconv"
	"strings"
)

// A group is one kind of a type's members, under a heading.
type group struct {
	name  string
	decls []*Decl
}

// groups are the type's members by kind, in the order a reader wants
// them: how to make one, then what it has.
func (t *Type) groups() []group {
	var out []group
	add := func(name string, ds []*Decl) {
		if len(ds) > 0 {
			out = append(out, group{name, ds})
		}
	}
	add("Cases", t.Cases)
	add("Initializers", t.Inits)
	var props, methods, other []*Decl
	for _, d := range t.Members {
		switch d.Kind {
		case "let", "var", "subscript":
			props = append(props, d)
		case "func":
			methods = append(methods, d)
		default:
			other = append(other, d)
		}
	}
	add("Properties", props)
	add("Methods", methods)
	add("Associated types and aliases", other)
	return out
}

// all is every member, in the order groups lists them.
func (t *Type) all() []*Decl {
	var out []*Decl
	for _, g := range t.groups() {
		out = append(out, g.decls...)
	}
	return out
}

// indexed is what an index lists of the type: its members but not its
// cases, which can run to hundreds in an enum of key codes and are read
// in the type's own entry.
func (t *Type) indexed() []*Decl {
	var out []*Decl
	for _, g := range t.groups() {
		if g.name != "Cases" {
			out = append(out, g.decls...)
		}
	}
	return out
}

// assignIDs gives every declaration a fragment identifier: its kind and
// name at the top level, its type's name and its own for a member, and a
// count after either for an overload.
func (p *Package) assignIDs() {
	used := map[string]int{}
	set := func(d *Decl, id string) {
		used[id]++
		if n := used[id]; n > 1 {
			id += "-" + strconv.Itoa(n)
		}
		d.ID = id
	}
	for _, list := range [][]*Decl{p.Consts, p.Vars, p.Funcs, p.Other} {
		for _, d := range list {
			set(d, d.Kind+"-"+slug(d.Name))
		}
	}
	for _, types := range [][]*Type{p.Types, p.Extensions} {
		for _, t := range types {
			set(t.Decl, t.Kind+"-"+slug(t.Name))
			for _, d := range t.all() {
				set(d, slug(t.Name)+"."+slug(d.Name))
			}
		}
	}
}

var slugRE = regexp.MustCompile(`[^A-Za-z0-9_.]+`)

func slug(s string) string {
	s = strings.Trim(s, "`")
	if out := slugRE.ReplaceAllString(s, "_"); strings.Trim(out, "_") != "" {
		return out
	}
	// An operator: its characters by name.
	var b strings.Builder
	for _, r := range s {
		b.WriteString("op")
		b.WriteString(strconv.Itoa(int(r)))
	}
	return b.String()
}

var spaceRE = regexp.MustCompile(`\s+`)

// oneLine is a signature on one line, for an index: attributes on lines
// of their own and the access level dropped, white space collapsed.
func oneLine(sig string) string {
	lines := strings.Split(sig, "\n")
	for len(lines) > 1 && strings.HasPrefix(strings.TrimSpace(lines[0]), "@") {
		lines = lines[1:]
	}
	s := spaceRE.ReplaceAllString(strings.Join(lines, " "), " ")
	s = strings.ReplaceAll(s, "( ", "(")
	s = strings.ReplaceAll(s, " )", ")")
	for _, access := range []string{"public ", "open "} {
		s = strings.Replace(s, access, "", 1)
	}
	return strings.TrimSpace(s)
}

// indexSig is a type's index line: its kind and name, and what it
// conforms to where that is short.
func indexSig(t *Type) string {
	s := oneLine(t.Sig)
	if len(s) > 90 {
		return t.Kind + " " + t.Name
	}
	return s
}
