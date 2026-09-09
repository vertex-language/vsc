// Euclid, as a loop.
func gcd(_ a: Int32, _ b: Int32) -> Int32 {
    var x = a
    var y = b
    while y != 0 {
        let t = y
        y = x % y
        x = t
    }
    return x
}

func main() -> Int32 {
    return gcd(1071, 462) + gcd(17, 5) * 10
}
