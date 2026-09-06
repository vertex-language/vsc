// Trial division, counting what survives.
func isPrime(_ n: Int32) -> Bool {
    if n < 2 { return false }
    var d: Int32 = 2
    while d * d <= n {
        if n % d == 0 { return false }
        d = d + 1
    }
    return true
}

func main() -> Int32 {
    var count: Int32 = 0
    for i in 0..<100 {
        if isPrime(Int32(i)) { count = count + 1 }
    }
    return count
}
