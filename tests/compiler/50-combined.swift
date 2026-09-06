// Several pieces at once, which is where they stop being isolated.
enum Op {
    case add
    case multiply
}

struct Pair {
    var a: Int32
    var b: Int32
}

final class Machine {
    var total: Int32 = 0

    func run(_ op: Op, _ p: Pair) -> Int32 {
        switch op {
        case .add: return p.a + p.b
        case .multiply: return p.a * p.b
        }
    }

    func accumulate(_ op: Op, _ p: Pair) {
        total = total + run(op, p)
    }
}

func main() -> Int32 {
    let m = Machine()
    let pairs = Pair(a: 3, b: 4)
    for _ in 0..<2 {
        m.accumulate(.add, pairs)
        m.accumulate(.multiply, pairs)
    }
    let scale: (Int32) -> Int32 = { $0 / 2 }
    return scale(m.total)
}
