// Instance methods, and mutating methods that change self.
struct Counter {
    var count = 0
    func doubled() -> Int { count * 2 }
    mutating func increment(by n: Int = 1) { count += n }
    mutating func reset() { self = Counter() }
}
var c = Counter()
c.increment()
c.increment(by: 4)
print(c.count, c.doubled())
c.reset()
print(c.count)
