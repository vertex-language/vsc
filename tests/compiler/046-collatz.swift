// Count the steps to reach one.
func steps(_ start: Int32) -> Int32 {
    var n = start
    var count: Int32 = 0
    while n != 1 {
        if n % 2 == 0 {
            n = n / 2
        } else {
            n = n * 3 + 1
        }
        count = count + 1
    }
    return count
}

func main() -> Int32 {
    return steps(27)
}
