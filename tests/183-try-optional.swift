// try? turns an error into nil; try! traps on one, so here it only succeeds.
enum E: Error { case no }
func half(_ n: Int) throws -> Int {
    if n % 2 != 0 { throw E.no }
    return n / 2
}
print(try? half(8) as Any, try? half(3) as Any)
print(try! half(10))
if let h = try? half(half(8)) { print("twice", h) }
let results = [4, 5, 6].map { try? half($0) }
print(results)
