// Hashable on a program's own types. A struct that says it is Hashable
// and writes no hash(into:) gets one combining every stored property, and
// an enum of cases alone gets one from its case. Set and Dictionary take
// such a type as an element or key, hashing and comparing it through its
// conformance -- its own == and hash(into:) where it writes them.
struct Point: Hashable {
    var x: Int
    var y: Int
}

struct Tagged: Hashable {
    let id: Int
    let note: String
    // Equal, and hashed, by id alone.
    static func == (a: Tagged, b: Tagged) -> Bool { return a.id == b.id }
    func hash(into hasher: inout Hasher) { hasher.combine(id) }
}

enum Color: Hashable {
    case red, green, blue
}

struct Wrapper: Hashable {
    let p: Point
    let c: Color
    let name: String
    let maybe: Int?
}

func main() -> Int32 {
    var failures: Int32 = 0

    var seen = Set<Point>()
    seen.insert(Point(x: 1, y: 2))
    seen.insert(Point(x: 1, y: 2))
    seen.insert(Point(x: 2, y: 1))
    if seen.count != 2 || !seen.contains(Point(x: 2, y: 1)) || seen.contains(Point(x: 3, y: 3)) { failures += 1 }

    var byTag: [Tagged: Int] = [:]
    byTag[Tagged(id: 1, note: "a")] = 10
    byTag[Tagged(id: 1, note: "b")] = 20
    byTag[Tagged(id: 2, note: "c")] = 30
    if byTag.count != 2 || byTag[Tagged(id: 1, note: "z")] != 20 { failures += 1 }

    let colors: Set<Color> = [.red, .blue, .red]
    if colors.count != 2 || !colors.contains(.blue) || colors.contains(.green) { failures += 1 }

    var names: [Wrapper: String] = [:]
    names[Wrapper(p: Point(x: 0, y: 0), c: .green, name: "w", maybe: nil)] = "first"
    names[Wrapper(p: Point(x: 0, y: 0), c: .green, name: "w", maybe: 1)] = "second"
    if names.count != 2 { failures += 1 }
    if names[Wrapper(p: Point(x: 0, y: 0), c: .green, name: "w", maybe: nil)] != "first" { failures += 1 }
    if names[Wrapper(p: Point(x: 0, y: 0), c: .red, name: "w", maybe: 1)] != nil { failures += 1 }

    var a = Hasher()
    a.combine(Point(x: 5, y: 6))
    var b = Hasher()
    b.combine(Point(x: 5, y: 6))
    if a.finalize() != b.finalize() { failures += 1 }

    print(failures == 0 ? "ok" : "failed \(failures)")
    return failures
}
