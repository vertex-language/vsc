// An error that reaches top-level code uncaught ends the program.
struct Fatal: Error {}
func risky(_ n: Int) throws -> Int {
    if n > 2 { throw Fatal() }
    return n
}
var total = 0
for n in 0..<5 {
    total += try risky(n)
}
