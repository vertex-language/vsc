package analyzer

import (
	"fmt"

	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/token"
)

type opItem struct {
	node   ast.Expr // *ast.OperatorExpr, *ast.TernaryExpr, or *ast.CastExpr
	group  *PrecedenceGroup
	isCast bool
	isTern bool
}

// FoldSequence folds a flat ast.SequenceExpr into a structured expression tree
// using the provided PrecedenceGraph.
func FoldSequence(f *token.File, seq *ast.SequenceExpr, pg *PrecedenceGraph) (ast.Expr, error) {
	if seq == nil || len(seq.Elements) == 0 {
		return nil, nil
	}
	if len(seq.Elements) == 1 {
		return seq.Elements[0], nil
	}

	var operands []ast.Expr
	var operators []opItem

	popAndApply := func() error {
		if len(operators) == 0 {
			return fmt.Errorf("unexpected empty operator stack")
		}
		top := operators[len(operators)-1]
		operators = operators[:len(operators)-1]

		if top.isTern {
			if len(operands) < 2 {
				return fmt.Errorf("insufficient operands for ternary operator")
			}
			elseExpr := operands[len(operands)-1]
			cond := operands[len(operands)-2]
			operands = operands[:len(operands)-2]

			tern := top.node.(*ast.TernaryExpr)
			condExpr := &ast.ConditionalExpr{
				Span:     ast.Span{Lo: cond.Pos(), Hi: elseExpr.End()},
				Cond:     cond,
				Question: tern.Question,
				Then:     tern.Then,
				Colon:    tern.Colon,
				Else:     elseExpr,
			}
			operands = append(operands, condExpr)
			return nil
		}

		if top.isCast {
			if len(operands) < 1 {
				return fmt.Errorf("insufficient operands for cast operator")
			}
			val := operands[len(operands)-1]
			operands = operands[:len(operands)-1]

			c := top.node.(*ast.CastExpr)
			castExpr := &ast.CastExpr{
				Span:     ast.Span{Lo: val.Pos(), Hi: c.End()},
				X:        val,
				Keyword:  c.Keyword,
				Kind:     c.Kind,
				Question: c.Question,
				Exclaim:  c.Exclaim,
				Type:     c.Type,
			}
			operands = append(operands, castExpr)
			return nil
		}

		// Binary operator
		if len(operands) < 2 {
			return fmt.Errorf("insufficient operands for binary operator")
		}
		rhs := operands[len(operands)-1]
		lhs := operands[len(operands)-2]
		operands = operands[:len(operands)-2]

		binOp := top.node.(*ast.OperatorExpr)
		binExpr := &ast.BinaryExpr{
			Span: ast.Span{Lo: lhs.Pos(), Hi: rhs.End()},
			X:    lhs,
			Op:   binOp,
			Y:    rhs,
		}
		operands = append(operands, binExpr)
		return nil
	}

	shouldPop := func(topOp, newOp opItem) bool {
		cmp := pg.Compare(topOp.group.Name, newOp.group.Name)
		if cmp > 0 {
			// topOp has higher precedence than newOp
			return true
		}
		if cmp == 0 {
			// equal precedence: pop if left-associative
			return topOp.group.Assoc == AssocLeft
		}
		return false
	}

	for _, elem := range seq.Elements {
		switch e := elem.(type) {
		case *ast.OperatorExpr:
			opName := string(f.Slice(e.Lo, e.Hi))
			grp := pg.OperatorGroup(opName)
			item := opItem{node: e, group: grp}

			for len(operators) > 0 && shouldPop(operators[len(operators)-1], item) {
				if err := popAndApply(); err != nil {
					return nil, err
				}
			}
			operators = append(operators, item)

		case *ast.TernaryExpr:
			grp := pg.LookupGroup("TernaryPrecedence")
			item := opItem{node: e, group: grp, isTern: true}

			for len(operators) > 0 && shouldPop(operators[len(operators)-1], item) {
				if err := popAndApply(); err != nil {
					return nil, err
				}
			}
			operators = append(operators, item)

		case *ast.CastExpr:
			grp := pg.LookupGroup("CastingPrecedence")
			item := opItem{node: e, group: grp, isCast: true}

			for len(operators) > 0 && shouldPop(operators[len(operators)-1], item) {
				if err := popAndApply(); err != nil {
					return nil, err
				}
			}
			operators = append(operators, item)
			// CastExpr doesn't have a right-side operand in the sequence; apply immediately
			if err := popAndApply(); err != nil {
				return nil, err
			}

		default:
			// Operand
			operands = append(operands, e)
		}
	}

	for len(operators) > 0 {
		if err := popAndApply(); err != nil {
			return nil, err
		}
	}

	if len(operands) != 1 {
		return nil, fmt.Errorf("sequence folding finished with %d operands", len(operands))
	}
	return operands[0], nil
}
