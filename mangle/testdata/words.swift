// Word substitutions.
//
// Swift's mangler splits an identifier into words and remembers each
// one that is at least two characters. A later identifier in the same
// symbol writes a back-reference where it repeats one, so the type
// `Pair` inside `sumPair(_:)` is not spelled again -- it is the word
// the function's own name already wrote down.
//
// The table is per symbol and the comparison is exact: `box` and
// `Box` are two words, which is why boxPair below substitutes only
// `Pair`.

public struct Pair { public var a: Int32 }
public struct PairBox { public var a: Int32 }
public struct AlphaBetaGamma { public var a: Int32 }
public struct HttpRequestHandler { public var a: Int32 }

// Every word of the type is already written: no text is left over.
func sumPair(_ p: Pair) -> Int32 { return p.a }
func sumPairBox(_ p: PairBox) -> Int32 { return p.a }

// Some words are, and the rest of the name follows them.
func sumPairAgain(_ p: PairBox) -> Int32 { return p.a }
func makeHttpRequest(_ p: HttpRequestHandler) -> Int32 { return p.a }

// The repeated word is not at the front, so text comes first.
func sumBox(_ p: PairBox) -> Int32 { return p.a }
func alphaGamma(_ p: AlphaBetaGamma) -> Int32 { return p.a }

// Nothing is shared, and the name is spelled out.
func betaThing(_ p: AlphaBetaGamma) -> Int32 { return p.a }
func longNameHere(_ p: Pair) -> Int32 { return p.a }

// Case is part of a word: `box` does not stand in for `Box`.
func boxPair(_ p: PairBox) -> Int32 { return p.a }
