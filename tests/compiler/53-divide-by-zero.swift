// Division by zero is a trap, not a value.
func opaque(_ n: Int32) -> Int32 { return n }

func main() -> Int32 {
    let a = opaque(10)
    let b = opaque(0)
    return a / b
}
