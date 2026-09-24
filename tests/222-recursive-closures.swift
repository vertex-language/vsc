// Recursion without a named function: local functions and a closure given itself.
func memoFib() -> (Int) -> Int {
    var memo: [Int: Int] = [:]
    func fib(_ n: Int) -> Int {
        if n < 2 { return n }
        if let v = memo[n] { return v }
        let v = fib(n - 1) + fib(n - 2)
        memo[n] = v
        return v
    }
    return fib
}
let fib = memoFib()
print(fib(10), fib(90))
func fix<A, B>(_ f: @escaping ((A) -> B, A) -> B) -> (A) -> B {
    func g(_ a: A) -> B { f(g, a) }
    return g
}
let fact = fix { (recur: (Int) -> Int, n: Int) in n <= 1 ? 1 : n * recur(n - 1) }
print(fact(10))
