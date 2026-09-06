// Int32.min / -1 has no representable answer.
func opaque(_ n: Int32) -> Int32 { return n }

func main() -> Int32 {
    let a = opaque(-2147483648)
    let b = opaque(-1)
    return a / b
}
