// An AsyncSequence of a program's own, consumed with for await.
struct Ticks: AsyncSequence {
    typealias Element = Int
    let count: Int
    struct AsyncIterator: AsyncIteratorProtocol {
        var n = 0
        let count: Int
        mutating func next() async -> Int? {
            guard n < count else { return nil }
            n += 1
            await Task.yield()
            return n
        }
    }
    func makeAsyncIterator() -> AsyncIterator { AsyncIterator(count: count) }
}
for await t in Ticks(count: 3) { print("tick", t) }
var stream = AsyncStream<Int> { c in
    for i in [5, 6, 7] { c.yield(i) }
    c.finish()
}
var sum = 0
for await v in stream { sum += v }
print(sum)
