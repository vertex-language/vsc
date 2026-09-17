// A function value whose parameter is too wide for registers. The call
// through it passes the value by address, which is what a direct call to
// the same function does; the signature a call through a function value
// uses had no room for one and refused to name a register for it.
struct Big {
    var a: Int
    var b: Int
    var c: Int
    var d: Int
    var name: String
}

func show(_ b: Big) -> Int {
    return b.a + b.b + b.c + b.d + b.name.count
}

func apply(_ n: Int, _ body: (Big) -> Int) -> Int {
    var total = 0
    var i = 1
    while i <= n {
        total += body(Big(a: i, b: i * 2, c: i * 3, d: i * 4, name: "a name long enough for the heap"))
        i += 1
    }
    return total
}

func main() -> Int32 {
    // A named function as the value, and a closure literal.
    let byName = apply(2, show)
    let byClosure = apply(2) { b in b.a + b.name.count }
    // And one stored in a variable first.
    let f: (Big) -> Int = show
    let stored = f(Big(a: 5, b: 6, c: 7, d: 8, name: "short"))
    return Int32((byName + byClosure + stored) % 250)
}
