// A Sequence of a program's own, with its IteratorProtocol, in a for loop.
struct Countdown: Sequence {
    let start: Int
    func makeIterator() -> Iterator { Iterator(n: start) }
    struct Iterator: IteratorProtocol {
        var n: Int
        mutating func next() -> Int? {
            guard n > 0 else { return nil }
            defer { n -= 1 }
            return n
        }
    }
}
for n in Countdown(start: 3) { print(n) }
print(Countdown(start: 5).map { $0 * 2 }, Countdown(start: 4).reduce(0, +))
var it = Countdown(start: 2).makeIterator()
print(it.next() as Any, it.next() as Any, it.next() as Any)
