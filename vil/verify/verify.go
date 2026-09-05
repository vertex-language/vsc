package verify

import "github.com/vertex-language/vsc/vil"

// Module checks every function in m and returns every fault found, or
// nil where there are none.
func Module(m *vil.Module) error {
	var all Errors
	for _, f := range m.Funcs() {
		if err := check(f); err != nil {
			all = append(all, err...)
		}
	}
	if len(all) == 0 {
		return nil
	}
	return all
}

// Func checks one function.
func Func(f *vil.Func) error {
	if errs := check(f); len(errs) > 0 {
		return errs
	}
	return nil
}

// stage checks the rules a module's stage carries. Raw VIL may hold
// instructions that only exist to be removed; canonical VIL may not,
// because the pass that removes them is what canonical means.
func (c *collector) stage(f *vil.Func) {
	m := f.Module()
	if m == nil || m.Stage() == vil.StageRaw {
		return
	}
	for _, b := range f.Blocks() {
		for i, in := range b.Insts() {
			if in.Op() == vil.MarkUninitialized {
				c.at(b, i, in.Op(), nil, ErrStage,
					"definite initialization removes it before "+string(m.Stage()))
			}
		}
	}
	if m.Stage() == vil.StageLowered && f.OSSA() {
		c.fnErr(ErrStage, "a lowered function is not in ownership form")
	}
}

func check(f *vil.Func) Errors {
	if f.IsDeclaration() {
		return nil // a declaration has no body to be wrong about
	}
	c := &collector{fn: f}
	d := buildDom(f)

	c.structure(f, d)
	if len(c.errs) > 0 {
		// A malformed graph makes dominance and the ownership rules
		// meaningless, and reporting what follows from a fault the
		// caller already has is noise.
		return c.errs
	}
	c.dominance(f, d)
	c.stage(f)
	c.ownership(f, d)
	return c.errs
}
