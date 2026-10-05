// Package analyzer performs semantic analysis on parsed ASTs: name resolution,
// operator precedence folding, type checking, member resolution, and literal decoding.
//
// Analysis proceeds in phases over all files in a module: precedence groups and
// operators, nominal types, members, extensions, protocols, and function bodies.
// Expressions that fail type checking are assigned types.Invalid to prevent cascading diagnostics.
package analyzer
