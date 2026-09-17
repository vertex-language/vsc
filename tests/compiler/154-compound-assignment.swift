// `x op= y`, which is `x = x op y`.
//
// Every one of them is that and nothing more: the same operator core
// declares, applied to what the destination holds and to the right
// operand, and stored back where it came from. So `+=` on an Int
// traps on overflow exactly as `+` does, and `+=` on a String
// allocates exactly as `+` does, because in both cases it is the same
// operator.
//
// The destination is read as a value and written as an address, which
// is two evaluations of the same expression. That is right for
// everything this compiler can assign to -- a local, a stored
// property, a field through self -- because none of them has a side
// effect to happen twice.
struct Counter { var n: Int32 }

class Box {
    var n: Int32 = 3
    func bump(_ k: Int32) { n += k }
}

func main() -> Int32 {
    // Arithmetic.
    var n: Int32 = 10
    n -= 3
    if n != 7 { return 91 }
    n *= 4
    if n != 28 { return 92 }
    n /= 2
    if n != 14 { return 93 }
    n %= 9
    if n != 5 { return 94 }
    n += 1
    if n != 6 { return 95 }

    // The bitwise ones, and the shifts.
    var m: Int32 = 0b1100
    m &= 0b1010
    if m != 8 { return 96 }
    m |= 0b0001
    if m != 9 { return 97 }
    m ^= 0b0010
    if m != 11 { return 98 }
    m <<= 2
    if m != 44 { return 99 }
    m >>= 1
    if m != 22 { return 100 }

    // A stored property, which is an address inside an address.
    var c = Counter(n: 5)
    c.n += 7
    if c.n != 12 { return 101 }

    // Through a class, and through self inside one.
    let b = Box()
    b.n *= 5
    if b.n != 15 { return 102 }
    b.bump(4)
    if b.n != 19 { return 103 }

    // Doubles, whose `+` is not a checked one.
    var d = 1.5
    d += 2.25
    if d != 3.75 { return 104 }

    return n * 7
}
