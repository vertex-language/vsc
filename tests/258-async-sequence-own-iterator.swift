// An AsyncSequence that is its own iterator, with a throwing next().
struct Countdown: AsyncSequence, AsyncIteratorProtocol {
    typealias Element = Int
    var n: Int
    mutating func next() async throws -> Int? {
        if n == 0 { return nil }
        defer { n -= 1 }
        return n
    }
    func makeAsyncIterator() -> Countdown { self }
}
var seen: [Int] = []
for try await x in Countdown(n: 4) { seen.append(x) }
print(seen)
