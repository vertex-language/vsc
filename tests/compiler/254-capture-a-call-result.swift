// A closure captures a value too wide for registers that has just come
// back from a call, so it is held as its scalars rather than in memory.
// The context needs its bytes, so they are written to the storage set
// aside for the value first, and copied from there.
//
// Capturing a parameter of the same type already worked: a parameter
// that wide arrives by address, and there was nothing to write out.
struct Big {
    var a: Int
    var b: Int
    var c: Int
    var d: Int
    var name: String
}

func make(_ n: Int) -> Big {
    return Big(a: n, b: n * 2, c: n * 3, d: n * 4, name: "a name long enough for the heap")
}

func sum(_ b: Big) -> Int {
    return b.a + b.b + b.c + b.d + b.name.count
}

func main() -> Int32 {
    // The captured value is make(2)'s result, never in memory of its own.
    let one = make(2)
    let f = { sum(one) }

    // And again where the result goes straight into the closure, with a
    // second capture beside it so the context holds more than one thing.
    let scale = 3
    let two = make(1)
    let g = { sum(two) * scale }

    return Int32((f() + g()) % 200)
}
