// Two functions that call each other, declared in either order.
func isEven(_ n: Int32) -> Bool {
    if n == 0 { return true }
    return isOdd(n - 1)
}

func isOdd(_ n: Int32) -> Bool {
    if n == 0 { return false }
    return isEven(n - 1)
}

func main() -> Int32 {
    var score: Int32 = 0
    if isEven(10) { score = score + 1 }
    if isOdd(7) { score = score + 2 }
    if !isEven(3) { score = score + 4 }
    return score
}
