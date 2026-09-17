// A case condition matches one enum case and binds what it carries, in if
// and while and beside other conditions; a let written before the pattern
// binds every name inside it, in a switch as well.
enum Shape {
    case circle(Int)
    case rect(Int, Int)
    case named(String)
    case none
}

enum Step {
    case more(Int)
    case done
}

func area(_ s: Shape) -> Int {
    if case .circle(let r) = s {
        return r * 3
    }
    if case let .rect(w, h) = s, w > 1 {
        return w * h
    }
    if case .named(let name) = s {
        return name.count
    }
    return 0
}

func describe(_ s: Shape) -> Int {
    switch s {
    case let .rect(w, h):
        return w + h
    case .circle(let r):
        return r
    case let .named(n):
        return n.count * 2
    case .none:
        return -1
    }
}

func next(_ n: Int) -> Step {
    if n > 0 {
        return .more(n)
    }
    return .done
}

func main() -> Int32 {
    var total = area(.circle(2)) + area(.rect(3, 4)) + area(.rect(1, 9))
    total += area(.named("hello")) + area(.none)
    total += describe(.rect(2, 5)) + describe(.circle(4)) + describe(.named("ab")) + describe(.none)
    var n = 4
    while case .more(let k) = next(n) {
        total += k
        n -= 1
    }
    if case .none = Shape.none {
        total += 100
    }
    return Int32(total % 251)
}
