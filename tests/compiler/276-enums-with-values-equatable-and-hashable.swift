// An enum whose cases carry values, and says it is Equatable or Hashable,
// gets == and hash(into:) from its cases: the same case with equal values
// is equal, and hashes alike. Set and Dictionary take it.
enum Shape: Hashable {
    case circle(Double)
    case rect(Int, Int)
    case named(String)
    case point
}

enum Token: Equatable {
    case word(String)
    case number(Int)
    case end
}

func main() -> Int32 {
    var failures: Int32 = 0
    if Shape.circle(1.5) != .circle(1.5) || Shape.circle(1) == .circle(2) { failures += 1 }
    if Shape.rect(1, 2) != .rect(1, 2) || Shape.rect(1, 2) == .rect(2, 1) { failures += 1 }
    if Shape.named("a") == .point || Shape.point != .point { failures += 1 }
    if Token.word("x") != .word("x") || Token.number(3) == .end { failures += 1 }

    var shapes = Set<Shape>()
    shapes.insert(.rect(3, 4))
    shapes.insert(.rect(3, 4))
    shapes.insert(.named("n"))
    shapes.insert(.point)
    if shapes.count != 3 || !shapes.contains(.named("n")) || shapes.contains(.rect(4, 3)) { failures += 1 }

    var areas: [Shape: Int] = [:]
    areas[.rect(2, 5)] = 10
    if areas[.rect(2, 5)] != 10 || areas[.circle(0)] != nil { failures += 1 }

    print([Token.word("hi"), .number(7), .end])
    print(failures == 0 ? "ok" : "failed \(failures)")
    return failures
}
