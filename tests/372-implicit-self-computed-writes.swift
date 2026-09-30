// A computed property named alone in a method or an initializer is
// written through its setter, as `self.name` is: assigned, compounded,
// mutated in place and through an element. An observed stored property
// named alone in its own didSet is written in place, as before.
final class Store {
    var storage: [Int] = []
    var writes = 0
    var items: [Int] {
        get { return storage }
        set { writes += 1; storage = newValue }
    }
    var count: Int {
        get { return storage.count }
        set { storage = Array(repeating: 0, count: newValue) }
    }
    var limit = 0 {
        didSet { if limit > 10 { limit = 10 } }
    }

    init(first: Int) {
        items = [first]
    }

    func add(_ x: Int) { items.append(x) }
    func bump(_ i: Int) { items[i] += 100 }
    func dropOdd() { items.removeAll { $0 % 2 != 0 } }
    func reset() { count = 2; limit = 50 }
}

struct Box {
    var raw = 1
    var doubled: Int {
        get { return raw * 2 }
        set { raw = newValue / 2 }
    }
    mutating func set(_ n: Int) { doubled = n; doubled += 4 }
}

let s = Store(first: 1)
s.add(2)
s.add(3)
s.bump(1)
print(s.items, s.writes)
s.dropOdd()
print(s.items, s.writes)
s.reset()
print(s.items, s.limit, s.writes)

var b = Box()
b.set(10)
print(b.raw, b.doubled)
