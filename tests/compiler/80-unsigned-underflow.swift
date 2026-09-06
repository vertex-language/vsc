// An unsigned subtraction below zero traps.
func opaque(_ n: UInt32) -> UInt32 { return n }

func main() -> Int32 {
    let a = opaque(1)
    let b = opaque(2)
    return Int32(a - b)
}
