// A mutating method is handed the receiver's storage, so what it
// writes is what the caller sees. Self used to cross the call by
// value, which left a mutating method writing to a copy -- the
// assignment was refused rather than lowered, and every program like
// this one was unbuildable.
struct Counter {
    var n: Int32

    mutating func bump() {
        n = n + 1
    }

    mutating func add(_ k: Int32) {
        self.n = self.n + k
    }

    // One mutating method calling another on the same receiver passes
    // the storage it was given, not a copy of it.
    mutating func bumpTwice() {
        bump()
        bump()
    }

    // An ordinary method lowered after a mutating one reads its
    // properties as a value, not through the other's address.
    func read() -> Int32 {
        return n
    }
}

func main() -> Int32 {
    var c = Counter(n: 0)
    c.bumpTwice()
    c.add(10)
    c.bump()
    return c.read()
}
