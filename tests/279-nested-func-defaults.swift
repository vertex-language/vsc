// A nested function that captures, called with its defaults left out.
struct E: Error {}
func outer(_ base: Int) throws -> [Int] {
    var out: [Int] = []
    func add(_ x: Int, twice: Bool = false, scale: Int = 1) throws {
        if x < 0 { throw E() }
        out.append((x + base) * scale)
        if twice { out.append(x) }
    }
    try add(1)
    try add(2, twice: true)
    try add(3, scale: 10)
    return out
}
print(try outer(100))
func plain() -> Int {
    func g(_ a: Int, b: Int = 5) -> Int { return a + b }
    return g(1) + g(1, b: 2)
}
print(plain())
