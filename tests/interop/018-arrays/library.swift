// An array literal, and an array across the boundary.
//
// One word: swiftc's own code for a function taking one reads x0 and
// nothing else, and what is in it is a reference to the storage the
// elements live in.
//
// Making one is three steps in SILGen and two here. The standard
// library allocates storage that has not been written to yet and
// hands back the array and a pointer to its elements; the elements
// are stored through that pointer; and then SILGen calls
// _finalizeUninitializedArray, which libswiftCore does not export and
// whose body is a word loaded and stored back. So there is nothing to
// call and nothing to do, which is what `swiftc -O` concludes too.
public func total(_ a: [Int32]) -> Int32 { a.reduce(0, &+) }

public func size(_ a: [Int32]) -> Int32 { Int32(a.count) }

public func at(_ a: [Int32], _ i: Int32) -> Int32 { a[Int(i)] }

public func totalWide(_ a: [Int64]) -> Int64 { a.reduce(0, &+) }

public func totalReal(_ a: [Double]) -> Double { a.reduce(0, +) }

public func trueCount(_ a: [Bool]) -> Int32 { Int32(a.filter { $0 }.count) }

// A variadic parameter, which is the same array said differently:
// the callee's signature is `(@guaranteed Array<Int32>) -> Int32` and
// knows nothing about how many arguments were written, and the symbol
// carries a `d` after the element type to say the list was one.
public func sum(_ xs: Int32...) -> Int32 { xs.reduce(0, &+) }

public func after(_ tag: Int32, _ xs: Int32...) -> Int32 {
    tag &* Int32(xs.count) &+ xs.reduce(0, &+)
}

// Handed back, which is the direction that allocates: what comes back
// owns its storage and the caller has to let go of it.
public func upTo(_ n: Int32) -> [Int32] { (0..<n).map { $0 } }
