// A closure written in a method may use self: by name, through a stored
// property or method -- named without it in a struct -- or listed as [self]. It holds a
// strong reference, so the object lives as long as the closure does,
// and the closure sees the object's later changes.
final class Counter {
    var hits = 0
    let step: Int
    var onEvent: (() -> Void)? = nil

    init(step: Int) {
        self.step = step
    }

    func bump() {
        hits += step
    }

    func arm() {
        onEvent = { self.bump() }
    }

    func reader() -> () -> Int {
        return { [self] in self.hits * 10 }
    }

    func adder() -> (Int) -> Int {
        return { x in x + self.hits + self.step }
    }

    func twice() -> () -> Void {
        return {
            self.bump()
            let inner = { self.hits += 1 }
            inner()
        }
    }
}

struct Scale {
    var factor: Int

    func apply(_ values: [Int]) -> Int {
        var total = 0
        for v in values {
            let scaled = { v * factor }
            total += scaled()
        }
        return total
    }
}

func main() -> Int32 {
    let c = Counter(step: 3)
    c.arm()
    if let e = c.onEvent {
        e()
        e()
    }
    c.onEvent = nil

    let read = c.reader()
    let add = c.adder()
    let t = c.twice()
    t()
    var total = read() + add(100)

    let scale = Scale(factor: 4)
    total += scale.apply([1, 2, 3])
    return Int32(total % 251)
}
