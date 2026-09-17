// Reading a class out of a stored property retains it, and something has to release it.
final class Leaf {
    var n: Int32 = 1
}

final class Branch {
    var leaf = Leaf()
    func value() -> Int32 { return leaf.n }
}

func main() -> Int32 {
    var total: Int32 = 0
    for _ in 0..<100 {
        let b = Branch()
        b.leaf.n = 42
        total = b.value()
        let held = b.leaf
        total = total + held.n - held.n
    }
    return total
}
