// A parameter the callee writes through.
//
// `inout` hands over the caller's storage rather than a copy of what
// is in it, so what crosses the call is an address: swiftc's own code
// for `swapped(_ a: inout Int32, _ b: inout Int32)` reads x0 and x1
// and stores through both.
//
// Which is also why an inout parameter is not a constant inside the
// body. Every other parameter is -- `n = 1` in a function taking `n:
// Int32` is an error, and Swift says so -- and writing to this one is
// the whole point of it.
struct Counter { var n: Int32 }

func swapped(_ a: inout Int32, _ b: inout Int32) { let t = a; a = b; b = t }
func bump(_ n: inout Int32) { n = n + 1 }
func bumpBy(_ n: inout Int32, _ k: Int32) { n = n + k }

// A field of a struct, which is an address inside an address.
func widen(_ c: inout Counter) { c.n = c.n * 2 }

// An inout parameter beside an ordinary one, which is where passing
// the value instead of the address shows up as a shifted argument
// list rather than as a lost write.
func mix(_ a: inout Int32, _ n: Int32, _ b: inout Int32) -> Int32 {
    a = a + n
    b = b + n
    return n
}

func main() -> Int32 {
    var a: Int32 = 2
    var b: Int32 = 42
    swapped(&a, &b)
    if a != 42 { return 91 }
    if b != 2 { return 92 }

    var c: Int32 = 41
    bump(&c)
    if c != 42 { return 93 }

    bumpBy(&c, 8)
    if c != 50 { return 94 }

    var box = Counter(n: 21)
    widen(&box)
    if box.n != 42 { return 95 }

    var x: Int32 = 1
    var y: Int32 = 2
    if mix(&x, 10, &y) != 10 { return 96 }
    if x != 11 { return 97 }
    if y != 12 { return 98 }

    return a
}
