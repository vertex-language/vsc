// A function that calls itself.
func factorial(_ n: Int) -> Int {
    n <= 1 ? 1 : n * factorial(n - 1)
}
func fib(_ n: Int) -> Int {
    n < 2 ? n : fib(n - 1) + fib(n - 2)
}
print(factorial(20), fib(25))
func depth(_ n: Int) -> Int { n == 0 ? 0 : 1 + depth(n - 1) }
print(depth(10000))
