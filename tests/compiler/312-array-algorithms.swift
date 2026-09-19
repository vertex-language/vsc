// Array's algorithms, written in the core as source and lowered where a
// program uses them, for the elements it uses them with: map, filter,
// reduce, compactMap, flatMap, forEach, contains(where:), allSatisfy,
// first(where:), last(where:), firstIndex, lastIndex, enumerated,
// reversed, sorted, sort, min, max, dropFirst, dropLast, prefix, suffix,
// drop(while:), prefix(while:); zip, stride, min, max, swap; an operator
// named as a value, `reduce(0, +)`; overloads told apart by their labels
// and by what a literal is on its own.

struct P: Equatable, Comparable {
    var n: Int
    static func < (a: P, b: P) -> Bool { return a.n < b.n }
}

func main() -> Int32 {
    let xs = [3, 1, 2, 5, 4]
    print(xs.map { $0 * 2 }, xs.filter { $0 % 2 == 1 }, xs.reduce(0) { $0 + $1 }, xs.reduce("") { $0 + String($1) })
    print(xs.compactMap { $0 > 2 ? $0 : nil }, xs.flatMap { [$0, $0] }, xs.contains { $0 > 4 }, xs.allSatisfy { $0 > 0 })
    print(xs.first { $0 > 2 } ?? -1, xs.last { $0 < 3 } ?? -1, xs.firstIndex { $0 == 2 } ?? -1, xs.lastIndex { $0 > 2 } ?? -1)
    for (i, x) in xs.enumerated() { print(i, x) }
    for x in xs.reversed() { print(x, terminator: " ") }
    print()
    print(Array(xs.reversed()), xs.sorted(), xs.sorted { $0 > $1 }, xs.min() ?? 0, xs.max() ?? 0)
    print(xs.min { $0 > $1 } ?? 0, xs.max { $0 > $1 } ?? 0)
    var ys = xs
    ys.sort()
    var zs = xs
    zs.sort { $0 > $1 }
    print(ys, zs, xs.dropFirst(), xs.dropFirst(2), xs.dropLast(), xs.prefix(2), xs.suffix(2), xs.prefix { $0 != 2 }, xs.drop { $0 != 2 })
    print(xs.firstIndex(of: 5) ?? -1, xs.lastIndex(of: 9) ?? -1, ["a", "b"].firstIndex(of: "b") ?? -1)
    print(min(3, 2), max(3, 2), min("b", "a"), max(2.5, 1.5), min(P(n: 1), P(n: 0)).n)
    for (a, b) in zip([1, 2, 3], ["x", "y"]) { print(a, b) }
    for i in stride(from: 0, to: 10, by: 3) { print(i, terminator: " ") }
    for i in stride(from: 10, through: 0, by: -5) { print(i, terminator: " ") }
    for d in stride(from: 0.0, to: 1.0, by: 0.25) { print(d, terminator: " ") }
    for d in stride(from: 0.5, through: 1.5, by: 0.5) { print(d, terminator: " ") }
    print()
    var a = 1, b = 2
    swap(&a, &b)
    print(a, b)
    xs.forEach { print($0, terminator: ",") }
    print()
    let ps = [P(n: 3), P(n: 1)]
    print(ps.sorted().map { $0.n }, ps.min()!.n, ps.firstIndex(of: P(n: 1)) ?? -1)
    let strs = ["bb", "a", "ccc"]
    print(strs.sorted(), strs.sorted { $0.count > $1.count }, strs.map { $0.count }.reduce(0, +))
    let f: (Int, Int) -> Int = (-)
    print(f(5, 1), [4, 2].reduce(1, *), [1, 2, 3].filter { $0 > 1 }.map { String($0) }.joined(separator: "-"), ["a", "b"].joined())
    return Int32(xs.reduce(0, +))
}
