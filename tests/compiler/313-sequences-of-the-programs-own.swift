// A for-in over a type of the program's own: through the iterator its
// makeIterator() makes, or the value itself where it is its own iterator
// -- next() asked before each pass, the loop over when it answers nil.
// A Sequence that is its own iterator has makeIterator() made for it,
// and its Element is what next() answers. Labels, where clauses,
// tuple elements and generic sequences.

struct Countdown: Sequence, IteratorProtocol {
    var n: Int
    mutating func next() -> Int? {
        if n <= 0 { return nil }
        n -= 1
        return n + 1
    }
}

struct Words: Sequence {
    let text: [String]
    func makeIterator() -> WordIterator { return WordIterator(words: text, i: 0) }
}

struct WordIterator: IteratorProtocol {
    let words: [String]
    var i: Int
    mutating func next() -> String? {
        if i >= words.count { return nil }
        i += 1
        return words[i - 1]
    }
}

final class Ticker: Sequence, IteratorProtocol {
    var n = 0
    func next() -> Int? { n += 1; return n > 3 ? nil : n }
}

struct Pairs<T>: Sequence, IteratorProtocol {
    var items: [T]
    var i = 0
    mutating func next() -> (T, T)? {
        if i + 1 >= items.count { return nil }
        i += 2
        return (items[i - 2], items[i - 1])
    }
}

func main() -> Int32 {
    for x in Countdown(n: 3) { print(x, terminator: " ") }
    print()
    for w in Words(text: ["a", "b", "c"]) where w != "b" { print(w) }
    for n in Ticker() { print(n, terminator: ",") }
    print()
    var seen = 0
    outer: for x in Countdown(n: 5) {
        for y in Countdown(n: x) {
            if y == 2 { continue outer }
            if x == 2 { break outer }
            print(x, y)
            seen += 1
        }
    }
    for (a, b) in Pairs(items: [1, 2, 3, 4, 5]) { print(a + b) }
    for (a, b) in Pairs(items: ["x", "y"]) { print(a + b) }
    let words = Words(text: ["p", "q"])
    for w in words { print(w) }
    for w in words { print(w, w) }
    return Int32(seen)
}
