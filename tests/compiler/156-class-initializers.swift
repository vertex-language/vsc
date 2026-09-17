// An `init` a class declares.
//
// A class's initializer is two functions where a struct's is one,
// which is what Swift's own `fC` and `fc` say: the allocating entry
// point makes the instance and the initializing one fills it in. They
// are separate because a subclass's initializer runs its superclass's
// on the same instance -- one allocation, two bodies.
//
// Both are emitted, under those two names. What differs from swiftc is
// the receiver of the allocating one: a class declared elsewhere is
// made by reading the instance's size out of the metadata, and one
// declared here is allocated by this compiler's own alloc_ref, whose
// size is known where it is written.
class Counter {
    var n: Int32
    var step: Int32

    init(n: Int32) {
        self.n = n
        self.step = 1
    }

    init(n: Int32, step: Int32) {
        self.n = n
        self.step = step
    }

    func bump() { n += step }
    func value() -> Int32 { return n }
}

// A property with no initial value, which is the case the free
// initializer could not make.
class Pair {
    var a: Int32
    var b: Int32
    init(_ a: Int32, _ b: Int32) { self.a = a; self.b = b }
    func sum() -> Int32 { return a + b }
}

// One whose body computes rather than copies.
class Doubled {
    var v: Int32
    init(of k: Int32) { v = k * 2 }
}

func main() -> Int32 {
    let c = Counter(n: 10)
    c.bump()
    if c.value() != 11 { return 91 }

    // The second initializer, told apart by its parameters.
    let d = Counter(n: 10, step: 5)
    d.bump()
    d.bump()
    if d.value() != 20 { return 92 }

    // A reference is a reference: two names for one object.
    let same = d
    same.bump()
    if d.value() != 25 { return 93 }

    if Pair(3, 4).sum() != 7 { return 94 }
    if Doubled(of: 8).v != 16 { return 95 }

    // Made inside a loop, which is a fresh object each time.
    var total: Int32 = 0
    var i: Int32 = 0
    while i < 4 {
        let p = Pair(i, i)
        total += p.sum()
        i += 1
    }
    if total != 12 { return 96 }

    return c.value() + d.value() + 6
}
