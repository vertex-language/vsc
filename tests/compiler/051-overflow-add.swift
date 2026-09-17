// Signed overflow traps rather than wrapping.
// The operand comes through a call so that it is a value at run time
// rather than a constant the compiler can fold and reject.
func opaque(_ n: Int32) -> Int32 { return n }

func main() -> Int32 {
    var n = opaque(2147483647)
    n = n + opaque(1)
    return n
}
