// A generic type's members in all their forms: methods with generic
// parameters of their own, computed properties read and written,
// extensions that add to it, and extensions that hold only where its
// parameters conform to something.

struct Stack<Element> {
    var items: [Element] = []

    mutating func push(_ e: Element) { items.append(e) }
    mutating func pop() -> Element? { if items.isEmpty { return nil }; return items.removeLast() }

    var count: Int { return items.count }
    var isEmpty: Bool { return items.isEmpty }
    var top: Element? {
        get { return items.last }
        set {
            if let v = newValue {
                if items.isEmpty { items.append(v) } else { items[items.count - 1] = v }
            }
        }
    }

    func map<U>(_ f: (Element) -> U) -> Stack<U> {
        var out = Stack<U>()
        for e in items { out.push(f(e)) }
        return out
    }
}

extension Stack {
    var depth: Int { return count }
    func peek(_ n: Int) -> Element { return items[items.count - 1 - n] }
    static func of(_ e: Element) -> Stack<Element> {
        var s = Stack<Element>()
        s.push(e)
        return s
    }
}

extension Stack where Element: Equatable {
    func contains(_ e: Element) -> Bool {
        for x in items where x == e { return true }
        return false
    }
    var allSame: Bool {
        guard let first = items.first else { return true }
        for x in items { if x != first { return false } }
        return true
    }
}

extension Stack where Element: Comparable {
    var largest: Element? {
        var best: Element? = nil
        for x in items {
            if let b = best {
                if x > b { best = x }
            } else {
                best = x
            }
        }
        return best
    }
}

final class Box<T> {
    var value: T
    init(_ value: T) { self.value = value }
    var described: String { return "Box(\(value))" }
}

extension Box where T: Comparable {
    func isBigger(than other: Box<T>) -> Bool { return value > other.value }
}

struct Pair<A, B> {
    var first: A
    var second: B
    var swapped: Pair<B, A> { return Pair<B, A>(first: second, second: first) }
}

func check(_ ok: Bool, _ n: Int32) -> Int32 { return ok ? 0 : n }

func main() -> Int32 {
    var failed: Int32 = 0
    var s = Stack<Int>()
    s.push(3)
    s.push(8)
    s.push(5)
    failed += check(s.count == 3 && !s.isEmpty && s.depth == 3, 1)
    failed += check(s.top == 5 && s.peek(1) == 8, 2)
    s.top = 7
    failed += check(s.top == 7 && s.count == 3, 3)
    failed += check(s.contains(8) && !s.contains(5), 4)
    failed += check(s.largest == 8 && !s.allSame, 5)
    failed += check(s.pop() == 7 && s.count == 2, 6)

    let words = s.map { "n\($0)" }
    failed += check(words.count == 2 && words.top == "n8" && words.contains("n3"), 7)
    failed += check(words.largest == "n8", 8)

    let one = Stack<String>.of("x")
    failed += check(one.count == 1 && one.allSame && one.top == "x", 9)

    let b = Box(4)
    b.value += 1
    failed += check(b.described == "Box(5)" && b.isBigger(than: Box(2)), 10)
    failed += check(Box("a").described == "Box(a)", 11)

    let p = Pair(first: 1, second: "one")
    failed += check(p.swapped.first == "one" && p.swapped.second == 1, 12)

    print(s.items, words.items, b.described, p.swapped.first)
    return failed
}
