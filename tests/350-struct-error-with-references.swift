// A struct holding references and strings, thrown as an Error and caught.
final class Source { let name: String; init(_ n: String) { name = n } }
struct ScopeError: Error {
    var message: String
    var pos: Int
    var source: Source
}
func check(_ n: Int) throws -> Int {
    if n < 0 { throw ScopeError(message: "negative", pos: n, source: Source("main.js")) }
    return n
}
do {
    print(try check(1))
    _ = try check(-4)
} catch let e as ScopeError {
    print(e.message, e.pos, e.source.name)
}
