struct Pair {
    var a: Int
    var b: Int
    init(a: Int, b: Int) {
        self.a = a
        self.b = b
    }
}
func use() -> Int {
    let p = Pair(a: 1, b: 2)
    return p.a + p.b
}
