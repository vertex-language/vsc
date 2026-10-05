// Any holds a value of any type; switch recovers it with is and as patterns.
let things: [Any] = [0, 3.5, "hi", (1, 2), [1, 2], Optional<Int>.none as Any, { (n: Int) in n + 1 }]
for t in things {
    switch t {
    case 0 as Int: print("zero")
    case let d as Double: print("double", d)
    case let s as String: print("string", s)
    case let (a, b) as (Int, Int): print("pair", a, b)
    case let xs as [Int]: print("array", xs.count)
    case let f as (Int) -> Int: print("function", f(1))
    default: print("other")
    }
}
