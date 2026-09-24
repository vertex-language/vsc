// A String across the boundary.
//
// Two words: swiftc's own code for `lengthOf` reads x0 and x1, and a
// function returning one leaves both. What is in them is Swift's
// business -- a short string is its own bytes, a longer one is a
// pointer biased by thirty-two with a bit that says immortal -- and
// this compiler never writes either encoding: it calls the standard
// library's own initializer, which is what SILGen does too.
public func lengthOf(_ s: String) -> Int32 { Int32(s.utf8.count) }

public func firstByte(_ s: String) -> Int32 { Int32(Array(s.utf8)[0]) }

public func lastByte(_ s: String) -> Int32 { Int32(Array(s.utf8).last!) }

// Handed back, which is the direction that allocates: this one is
// longer than fits inline, so what comes back owns a heap object and
// the caller has to let go of it.
public func repeated(_ s: String, _ n: Int32) -> String {
    String(repeating: s, count: Int(n))
}

public func echo(_ s: String) -> String { s }

public func isEmptyString(_ s: String) -> Int32 { s.isEmpty ? 1 : 0 }
