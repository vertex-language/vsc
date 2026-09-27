// A nested function that captures and calls itself.
func count(_ n: Int) -> [Int] {
    var out: [Int] = []
    func go(_ i: Int) {
        if i > n { return }
        out.append(i)
        go(i + 1)
    }
    go(1)
    return out
}
print(count(4))
final class C {
    var depth = 3
    func walk() -> Int {
        func step(_ d: Int) -> Int { d == 0 ? 0 : 1 + step(d - 1) + depth - depth }
        return step(depth)
    }
}
print(C().walk())
