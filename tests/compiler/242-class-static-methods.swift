// A class's static methods are called on the class: one that makes an
// instance, and one that works on plain values.
final class Counter {
    var n: Int
    init(n: Int) {
        self.n = n
    }
    static func make(_ start: Int) -> Counter {
        return Counter(n: start + 1)
    }
    static func twice(_ x: Int) -> Int {
        return x * 2
    }
}

func main() -> Int32 {
    let c = Counter.make(10)
    return Int32(c.n + Counter.twice(4))
}
