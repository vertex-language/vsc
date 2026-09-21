// A receiver copied to call a non-mutating method ends with that call:
// write(above()) inside a mutating method, chained calls on temporaries,
// and a let whose value is the receiver and is used again afterwards.

struct Grid {
    var cells: [Int]
    var at: Int = 0
    func above() -> Int { return at >= 2 ? cells[at - 2] : 0 }
    func peek() -> Int { return cells[at] }
    mutating func write(_ v: Int) { cells[at] = v; at += 1 }
    mutating func fill() {
        while at < cells.count { write(above() + 1) }
    }
    mutating func fillSelf() {
        at = 0
        while at < cells.count { self.write(self.above() * 2 + peek()) }
    }
}

struct Box {
    var items: [String]
    func first() -> String { return items[0] }
    func doubled() -> Box { return Box(items: items + items) }
    func count() -> Int { return items.count }
}

final class Counter {
    var n = 0
    func next() -> Int { n += 1; return n }
}

struct Holder {
    let c: Counter
    var log: [Int] = []
    func tick() -> Int { return c.next() }
    mutating func record() { log.append(tick()) }
}

func main() -> Int32 {
    var g = Grid(cells: [Int](repeating: 0, count: 8))
    g.fill()
    print(g.cells)
    g.fillSelf()
    print(g.cells)

    let b = Box(items: ["a", "b"])
    print(b.doubled().doubled().count(), b.doubled().first())
    print(b.first(), b.count())

    var h = Holder(c: Counter())
    h.record(); h.record(); h.record()
    print(h.log, h.c.n)
    return 0
}
