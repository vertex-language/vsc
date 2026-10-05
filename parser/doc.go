// Package parser implements a recursive descent parser for Swift source code,
// producing an *ast.File and diagnostics from a *token.File.
//
// Expressions remain unfolded in SequenceExpr nodes for later precedence folding
// by the semantic analyzer. Ambiguous constructs are resolved via backtracking or
// basic mode parsing when followed by statement bodies.
package parser
