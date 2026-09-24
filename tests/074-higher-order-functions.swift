// Functions that take and build other functions.
func compose<A, B, C>(_ f: @escaping (A) -> B, _ g: @escaping (B) -> C) -> (A) -> C {
    { g(f($0)) }
}
func adder(_ n: Int) -> (Int) -> Int { { $0 + n } }
let addThenDouble = compose(adder(3), { $0 * 2 })
let toText = compose(addThenDouble, { "result \($0)" })
print(addThenDouble(4), toText(10))
