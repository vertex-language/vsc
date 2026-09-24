// A subscript declared with a getter alone cannot be assigned through.
struct S {
    var xs = [1, 2]
    subscript(i: Int) -> Int { return xs[i] }
}

func f() {
    var s = S()
    s[0] = 3
}
