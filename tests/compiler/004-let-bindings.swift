// A constant is the value it was given, and shadowing is a new one.
func main() -> Int32 {
    let a: Int32 = 10
    let b = a + 5
    let c = b + a
    return c
}
