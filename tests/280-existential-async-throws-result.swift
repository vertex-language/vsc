// A class-bound existential an async throwing function returns, bound
// to a let and called; and one a stored async closure returns.
protocol M: AnyObject {
    func Forward(_ t: Int) async throws -> Int
}
final class L: M {
    func Forward(_ t: Int) async throws -> Int { return t + 1 }
}
func make() async throws -> any M { return L() }
func run() async throws {
    let a = try await make()
    print(try await a.Forward(1))
}
let load: () async throws -> any M = { return L() }
func run2() async throws { let b = try await load(); print(try await b.Forward(5)) }
try await run()
try await run2()
