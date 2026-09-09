struct Counter {
    var n = 0
    mutating func bump() { n += 1 }
    var doubled: Int { return n * 2 }
    subscript(i: Int) -> Int { return n + i }
}
func use(_ c: inout Counter) -> Int {
    c.bump()
    return c.doubled + c[1]
}
