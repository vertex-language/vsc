// Functions returning functions: currying and partial application.
func add(_ a: Int) -> (Int) -> (Int) -> Int { { b in { c in a + b + c } } }
func curry<A, B, C>(_ f: @escaping (A, B) -> C) -> (A) -> (B) -> C { { a in { b in f(a, b) } } }
let add5 = add(2)(3)
print(add5(10), add(1)(1)(1))
let pow2 = curry { (base: Int, e: Int) -> Int in (0..<e).reduce(1) { acc, _ in acc * base } }(2)
print((0...5).map(pow2))
