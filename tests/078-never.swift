// A function returning Never does not come back: fatalError ends the program.
func fail(_ why: String) -> Never {
    fatalError(why)
}
func check(_ n: Int) -> Int {
    guard n > 0 else { fail("n was \(n)") }
    return n
}
var total = 0
for n in [3, 2, 1, 0] {
    total += check(n)
}
