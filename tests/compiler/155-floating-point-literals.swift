// A literal with a fraction in it.
//
// What reaches a register is the bits, which is how SIL writes one
// too -- `float_literal $Builtin.FPIEEE64, 0x3FF8000000000000` -- and
// the width they are is the type's. A Double literal rounds once, in
// the scanner; a Float literal rounds again, here, rather than
// somewhere further down where nothing would say it had.
//
// The arithmetic itself was already here: core declares `+` on Double
// as `fadd_FPIEEE64` and has since the operator table was written.
// What was missing was any way to write a number for it to work on.
func half() -> Double { return 0.5 }
func third() -> Float { return 0.25 }

struct Circle { var r: Double }

func area(_ c: Circle) -> Double { return c.r * c.r * 3.0 }

func main() -> Int32 {
    // The four operations, at both widths.
    if 1.5 + 2.25 != 3.75 { return 91 }
    if 5.5 - 0.5 != 5.0 { return 92 }
    if 1.5 * 4.0 != 6.0 { return 93 }
    if 7.5 / 2.5 != 3.0 { return 94 }

    // The orderings, which are the ordered comparisons.
    if !(1.5 < 2.0) { return 95 }
    if 2.0 < 1.5 { return 96 }
    if !(2.0 >= 2.0) { return 97 }
    if 1.0 == 2.0 { return 98 }
    if !(1.0 != 2.0) { return 99 }

    // A sign in front is one constant and not an operator applied to
    // another.
    let below: Double = -1.25
    if below + 1.25 != 0.0 { return 100 }

    // Through a binding, a function, and a field.
    let h = half()
    if h * 2.0 != 1.0 { return 101 }
    let t = third()
    if t * 4.0 != 1.0 { return 102 }
    if area(Circle(r: 2.0)) != 12.0 { return 103 }

    // A whole number written as one: 2.0 is a Double and not an Int.
    var d = 2.0
    d += 1.0
    if d != 3.0 { return 104 }

    return 42
}
