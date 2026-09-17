package verify

import "github.com/vertex-language/vsc/internal/sil"

// Module checks every function in m and returns all faults found.
func Module(m *sil.Module) error {
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
func Func(f *sil.Func) error {
	if errs := check(f); len(errs) > 0 {
		return errs
	}
	return nil
}

// stage checks stage-specific invariants (e.g. no mark_uninitialized in canonical SIL).
func (c *collector) stage(f *sil.Func) {
	m := f.Module()
	if m == nil || m.Stage() == sil.StageRaw {
		return
	}
	for _, b := range f.Blocks() {
		for i, in := range b.Insts() {
			if in.Op() == sil.MarkUninitialized {
				c.at(b, i, in.Op(), nil, ErrStage,
					"definite initialization removes it before "+string(m.Stage()))
			}
		}
	}
	if m.Stage() == sil.StageLowered && f.OSSA() {
		c.fnErr(ErrStage, "a lowered function is not in ownership form")
	}
}

func check(f *sil.Func) Errors {
	if f.IsDeclaration() {
		return nil
	}
	c := &collector{fn: f}
	d := buildDom(f)

	c.structure(f, d)
	if len(c.errs) > 0 {
		// Abort further verification if structural checks failed.
		return c.errs
	}
	c.dominance(f, d)
	c.stage(f)
	c.ownership(f, d)
	return c.errs
}
