// Package token defines the lexical vocabulary of the Vertex source
// language — Swift 6 syntax, as given in docs/swift_grammar.md — and
// the per-file position space every span in the front end resolves
// through.
//
// Invariants:
//
//  1. Nothing below the parser interprets. Tokens carry no text;
//     literals arrive undecoded and resolve through the File that
//     produced them.
//  2. No cross-file address space. A Pos is per-File.
//  3. Every span is non-empty (End > Pos), including ILLEGAL. The
//     scanner's EOF token is the one zero-width exception.
//
// Two facts about Swift shape this package, and both are absences.
//
// There is no fixed operator table: an operator is a run of operator
// characters, and what it means — its precedence, its associativity,
// whether it exists at all — is decided by a precedencegroup
// declaration the analyzer resolves. So there is no Precedence method
// here and no per-operator Kind. An operator token is one of
// OPER_PREFIX, OPER_BINARY, or OPER_POSTFIX, and its spelling is its
// span. The scanner picks among the three by the whitespace around
// the run, which is the only thing that can be known lexically.
//
// There is no preprocessor: #if is a statement in the grammar, so
// conditional compilation reaches the parser as tokens like any other
// construct, and this package never sees a translated buffer. The
// text the scanner reads is the text the user typed.
package token
