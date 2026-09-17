// Extensions of the standard library's own types: Int, String, Double,
// Bool, Array, Dictionary, Set and Optional given methods, properties,
// static members and conformances, used directly, through generics and
// inside existentials.

protocol Named {
    var name: String { get }
    func weight() -> Int
}

extension Int: Named {
    var name: String { return "int \(self)" }
    func weight() -> Int { return self }

    var doubled: Int { return self * 2 }
    var isEven: Bool { return self % 2 == 0 }
    func times(_ body: () -> Void) {
        var i = 0
        while i < self { body(); i += 1 }
    }
    mutating func bump(by n: Int = 1) { self += n }
    static func parsed(_ s: String) -> Int { return Int(s) ?? -1 }
    static var answer: Int { return 42 }
}

extension String: Named {
    var name: String { return "string " + self }
    func weight() -> Int { return count }

    var shouted: String { return self + "!" }
    func repeated(_ n: Int) -> String {
        var out = ""
        for _ in 0..<n { out += self }
        return out
    }
}

extension Double {
    var half: Double { return self / 2 }
}

extension Bool {
    var flipped: Bool { return !self }
}

extension Array {
    var second: Element? { return count > 1 ? self[1] : nil }
    func mapped<U>(_ f: (Element) -> U) -> [U] {
        var out: [U] = []
        for x in self { out.append(f(x)) }
        return out
    }
    func kept(_ keep: (Element) -> Bool) -> [Element] {
        var out: [Element] = []
        for x in self where keep(x) { out.append(x) }
        return out
    }
    mutating func takeLast() -> Element? {
        if isEmpty { return nil }
        return removeLast()
    }
}

extension Array where Element: Equatable {
    func occurrences(of e: Element) -> Int {
        var n = 0
        for x in self where x == e { n += 1 }
        return n
    }
}

extension Array where Element: Comparable {
    var largest: Element? {
        guard var best = first else { return nil }
        for x in self where x > best { best = x }
        return best
    }
}

extension Dictionary {
    var size: Int { return count }
    func value(_ key: Key, or fallback: Value) -> Value { return self[key] ?? fallback }
}

extension Set {
    var many: Bool { return count > 1 }
}

extension Optional {
    var isSome: Bool {
        switch self {
        case .some: return true
        case .none: return false
        }
    }
    func or(_ fallback: Wrapped) -> Wrapped { return self ?? fallback }
}

func describe<T: Named>(_ x: T) -> String { return "\(x.name)/\(x.weight())" }

func check(_ ok: Bool, _ n: Int32) -> Int32 { return ok ? 0 : n }

func main() -> Int32 {
    var failed: Int32 = 0

    var n = 3
    n.bump()
    n.bump(by: 2)
    failed += check(n == 6 && n.doubled == 12 && n.isEven && 7.doubled == 14, 1)
    var calls = 0
    3.times { calls += 1 }
    failed += check(calls == 3, 2)
    failed += check(Int.parsed("12") == 12 && Int.parsed("x") == -1 && Int.answer == 42, 3)

    failed += check("hi".shouted == "hi!" && "ab".repeated(3) == "ababab", 4)
    failed += check(3.0.half == 1.5 && true.flipped == false, 5)

    var xs = [4, 1, 4, 9]
    failed += check(xs.second == 1 && [7].second == nil, 6)
    failed += check(xs.mapped { $0 * 10 } == [40, 10, 40, 90], 7)
    failed += check(xs.kept { $0 > 2 } == [4, 4, 9], 8)
    failed += check(xs.occurrences(of: 4) == 2 && xs.largest == 9, 9)
    failed += check(xs.takeLast() == 9 && xs.count == 3, 10)
    failed += check(["pear", "fig"].largest == "pear" && ([] as [Int]).largest == nil, 11)
    failed += check(xs.mapped { "\($0)" }.occurrences(of: "4") == 2, 12)

    let d = ["a": 1, "b": 2]
    failed += check(d.size == 2 && d.value("a", or: 0) == 1 && d.value("z", or: 7) == 7, 13)
    let pair: Set<Int> = [1, 2]
    let one: Set<Int> = [1]
    failed += check(pair.many && !one.many, 14)

    let some: Int? = 5
    let none: String? = nil
    failed += check(some.isSome && !none.isSome && some.or(0) == 5 && none.or("x") == "x", 15)

    failed += check(describe(8) == "int 8/8" && describe("abc") == "string abc/3", 16)
    let things: [Named] = [2, "xy", 10]
    var total = 0
    for t in things { total += t.weight() }
    failed += check(total == 14 && things[1].name == "string xy", 17)

    print(n, n.doubled, "hi".shouted, xs.mapped { $0 + 1 }, d.size, some.or(0), describe(8))
    for t in things { print(t.name) }
    return failed
}
