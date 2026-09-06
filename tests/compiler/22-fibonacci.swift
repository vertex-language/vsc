// Two recursive calls per step.
func fib(_ n: Int32) -> Int32 {
    if n < 2 { return n }
    return fib(n - 1) + fib(n - 2)
}

func main() -> Int32 {
    return fib(10)
}
