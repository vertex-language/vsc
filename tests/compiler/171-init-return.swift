// `return` in an initializer leaves early with what has been built.
// It was checked against the type being made, so a bare return was a
// conversion error -- Void where the type was wanted -- and the
// initializer's one legal return statement did not compile.
//
// swiftc is exact about this: "'nil' is the only return value
// permitted in an initializer". So the return takes no value, and
// lowering gives it the same thing the end of the body gives.
struct Clamped {
    var n: Int32

    init(_ v: Int32) {
        self.n = v
        if v > 10 { return }
        self.n = v * 2
    }
}

final class Counter {
    var n: Int32

    init(_ v: Int32) {
        self.n = v
        return
    }
}

func main() -> Int32 {
    return Clamped(20).n + Clamped(3).n + Counter(4).n
}
