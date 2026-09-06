// and, or, exclusive or, and complement.
func main() -> Int32 {
    let a: Int32 = 0b1100
    let b: Int32 = 0b1010
    let and = a & b
    let or = a | b
    let xor = a ^ b
    return and + or + xor
}
