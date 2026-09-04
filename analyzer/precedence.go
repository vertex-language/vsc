package analyzer

import (
	"fmt"
)

// Associativity determines operator binding direction for equal precedence.
type Associativity int

const (
	AssocNone Associativity = iota
	AssocLeft
	AssocRight
)

// PrecedenceGroup is what decides how an operator binds. Swift has no
// fixed table: a program declares its groups and says which is higher
// than which, and the folding order is read off the graph.
type PrecedenceGroup struct {
	Name       string
	HigherThan []string
	LowerThan  []string
	Assoc      Associativity
	Assignment bool
}

// PrecedenceGraph stores declared groups and computes relative precedence.
type PrecedenceGraph struct {
	groups    map[string]*PrecedenceGroup
	operators map[string]string // operator spelling -> group name
}

// NewPrecedenceGraph creates a new PrecedenceGraph initialized with standard Swift groups.
func NewPrecedenceGraph() *PrecedenceGraph {
	pg := &PrecedenceGraph{
		groups:    make(map[string]*PrecedenceGroup),
		operators: make(map[string]string),
	}
	pg.initBuiltins()
	return pg
}

func (pg *PrecedenceGraph) initBuiltins() {
	// Builtin precedence groups
	pg.AddGroup(&PrecedenceGroup{
		Name:       "AssignmentPrecedence",
		Assoc:      AssocRight,
		Assignment: true,
	})
	pg.AddGroup(&PrecedenceGroup{
		Name:       "TernaryPrecedence",
		HigherThan: []string{"AssignmentPrecedence"},
		Assoc:      AssocRight,
	})
	pg.AddGroup(&PrecedenceGroup{
		Name:       "LogicalDisjunctionPrecedence",
		HigherThan: []string{"TernaryPrecedence"},
		Assoc:      AssocLeft,
	})
	pg.AddGroup(&PrecedenceGroup{
		Name:       "LogicalConjunctionPrecedence",
		HigherThan: []string{"LogicalDisjunctionPrecedence"},
		Assoc:      AssocLeft,
	})
	pg.AddGroup(&PrecedenceGroup{
		Name:       "ComparisonPrecedence",
		HigherThan: []string{"LogicalConjunctionPrecedence"},
		Assoc:      AssocNone,
	})
	pg.AddGroup(&PrecedenceGroup{
		Name:       "NilCoalescingPrecedence",
		HigherThan: []string{"ComparisonPrecedence"},
		Assoc:      AssocRight,
	})
	pg.AddGroup(&PrecedenceGroup{
		Name:       "RangeFormationPrecedence",
		HigherThan: []string{"ComparisonPrecedence"},
		Assoc:      AssocNone,
	})
	pg.AddGroup(&PrecedenceGroup{
		Name:       "CastingPrecedence",
		HigherThan: []string{"RangeFormationPrecedence"},
		Assoc:      AssocNone,
	})
	pg.AddGroup(&PrecedenceGroup{
		Name:       "AdditionPrecedence",
		HigherThan: []string{"RangeFormationPrecedence"},
		Assoc:      AssocLeft,
	})
	pg.AddGroup(&PrecedenceGroup{
		Name:       "MultiplicationPrecedence",
		HigherThan: []string{"AdditionPrecedence"},
		Assoc:      AssocLeft,
	})
	pg.AddGroup(&PrecedenceGroup{
		Name:       "BitwiseShiftPrecedence",
		HigherThan: []string{"MultiplicationPrecedence"},
		Assoc:      AssocNone,
	})

	// Builtin operator mappings
	ops := map[string]string{
		"=":   "AssignmentPrecedence",
		"+=":  "AssignmentPrecedence",
		"-=":  "AssignmentPrecedence",
		"*=":  "AssignmentPrecedence",
		"/=":  "AssignmentPrecedence",
		"||":  "LogicalDisjunctionPrecedence",
		"&&":  "LogicalConjunctionPrecedence",
		"==":  "ComparisonPrecedence",
		"!=":  "ComparisonPrecedence",
		"<":   "ComparisonPrecedence",
		"<=":  "ComparisonPrecedence",
		">":   "ComparisonPrecedence",
		">=":  "ComparisonPrecedence",
		"===": "ComparisonPrecedence",
		"!==": "ComparisonPrecedence",
		"??":  "NilCoalescingPrecedence",
		"...": "RangeFormationPrecedence",
		"..<": "RangeFormationPrecedence",
		"+":   "AdditionPrecedence",
		"-":   "AdditionPrecedence",
		"*":   "MultiplicationPrecedence",
		"/":   "MultiplicationPrecedence",
		"%":   "MultiplicationPrecedence",
		"<<":  "BitwiseShiftPrecedence",
		">>":  "BitwiseShiftPrecedence",
	}
	for op, grp := range ops {
		pg.AddOperator(op, grp)
	}
}

// AddGroup registers or updates a precedence group.
func (pg *PrecedenceGraph) AddGroup(g *PrecedenceGroup) {
	pg.groups[g.Name] = g
}

// AddOperator maps an operator symbol to a precedence group name.
func (pg *PrecedenceGraph) AddOperator(op string, group string) {
	pg.operators[op] = group
}

// LookupGroup returns the precedence group by name.
func (pg *PrecedenceGraph) LookupGroup(name string) *PrecedenceGroup {
	return pg.groups[name]
}

// OperatorGroup returns the precedence group for op, defaulting to DefaultPrecedence if unknown.
func (pg *PrecedenceGraph) OperatorGroup(op string) *PrecedenceGroup {
	grpName, ok := pg.operators[op]
	if !ok {
		// Fallback for custom undefined operators: default to ComparisonPrecedence
		return pg.groups["ComparisonPrecedence"]
	}
	return pg.groups[grpName]
}

// HigherReports reports whether group g1 is strictly higher precedence than g2.
func (pg *PrecedenceGraph) HigherThan(g1, g2 string) bool {
	if g1 == g2 || g1 == "" || g2 == "" {
		return false
	}
	// BFS reachability from g1 along HigherThan edges
	visited := make(map[string]bool)
	queue := []string{g1}
	visited[g1] = true

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]

		grp := pg.groups[curr]
		if grp == nil {
			continue
		}
		for _, h := range grp.HigherThan {
			if h == g2 {
				return true
			}
			if !visited[h] {
				visited[h] = true
				queue = append(queue, h)
			}
		}
	}
	return false
}

// Compare returns:
//
//	> 0 if g1 has higher precedence than g2
//	< 0 if g2 has higher precedence than g1
//	0 if g1 and g2 are the same group or mutually incomparable
func (pg *PrecedenceGraph) Compare(g1, g2 string) int {
	if g1 == g2 {
		return 0
	}
	if pg.HigherThan(g1, g2) {
		return 1
	}
	if pg.HigherThan(g2, g1) {
		return -1
	}
	return 0
}

func (pg *PrecedenceGraph) String() string {
	return fmt.Sprintf("PrecedenceGraph(%d groups, %d ops)", len(pg.groups), len(pg.operators))
}
