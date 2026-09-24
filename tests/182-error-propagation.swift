// An error passes up through throwing functions until something catches it.
struct Failure: Error { let depth: Int }
func level(_ n: Int) throws -> Int {
    if n == 0 { throw Failure(depth: 3) }
    return try level(n - 1) + 1
}
func top() throws -> Int {
    defer { print("top unwinding") }
    return try level(3)
}
do {
    _ = try top()
} catch let f as Failure {
    print("caught at depth", f.depth)
}
