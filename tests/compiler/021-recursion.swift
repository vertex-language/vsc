// A function that calls itself, and a base case that stops it.
func factorial(_ n: Int32) -> Int32 {
    if n <= 1 { return 1 }
    return n * factorial(n - 1)
}

func main() -> Int32 {
    return factorial(5)
}
