// Exponentiation by squaring.
func power(_ base: Int32, _ exp: Int32) -> Int32 {
    var result: Int32 = 1
    var b = base
    var e = exp
    while e > 0 {
        if e % 2 == 1 { result = result * b }
        b = b * b
        e = e / 2
    }
    return result
}

func main() -> Int32 {
    return power(2, 5) + power(3, 2) + power(5, 0)
}
