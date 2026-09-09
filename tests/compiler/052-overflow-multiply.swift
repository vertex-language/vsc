// So does a product that does not fit.
func opaque(_ n: Int32) -> Int32 { return n }

func main() -> Int32 {
    let a = opaque(100000)
    let b = opaque(100000)
    return a * b
}
