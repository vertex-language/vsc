// A struct held in a `let` cannot be written through its subscript.
struct S {
    var xs = [1, 2]
    subscript(i: Int) -> Int {
        get { return xs[i] }
        set { xs[i] = newValue }
    }
}

func f() {
    let s = S()
    s[0] = 3
}
