// rethrows throws only when its closure argument does.
func applyAll(_ xs: [Int], _ f: (Int) throws -> Int) rethrows -> [Int] {
    var out: [Int] = []
    for x in xs { out.append(try f(x)) }
    return out
}
struct TooBig: Error { let value: Int }
print(applyAll([1, 2, 3]) { $0 * 2 })
do {
    _ = try applyAll([1, 20, 3]) { x in
        if x > 10 { throw TooBig(value: x) }
        return x
    }
} catch let e as TooBig {
    print("too big", e.value)
}
print(try [1, 2].map { x throws -> Int in x + 1 })
