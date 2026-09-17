// Equatable and Comparable on a program's own types. A struct or enum
// that says it is Equatable, and declares no `==` of its own, gets one
// comparing every stored property or the case. `!=` comes with `==`, and
// `>`, `<=` and `>=` come with `<`. A type that writes its own `==` has
// that one used.
struct Point: Equatable {
    var x: Int
    var y: Int
}

struct Span: Comparable {
    let nanos: Int64
    static func < (a: Span, b: Span) -> Bool { return a.nanos < b.nanos }
}

enum Light: Equatable {
    case red, amber, green
}

struct Loose: Equatable {
    let value: Int
    let note: String
    // Equal whatever the note says.
    static func == (a: Loose, b: Loose) -> Bool { return a.value == b.value }
}

struct Pair: Equatable {
    let a: Point
    let b: Span
    let name: String
    let when: Double
}

func main() -> Int32 {
    var failures: Int32 = 0
    let p = Point(x: 1, y: 2)
    if !(p == Point(x: 1, y: 2)) { failures += 1 }
    if p == Point(x: 1, y: 3) { failures += 1 }
    if !(p != Point(x: 2, y: 2)) { failures += 1 }

    let one = Span(nanos: 1)
    let two = Span(nanos: 2)
    if !(one < two) || one > two || !(two > one) { failures += 1 }
    if !(one <= one) || !(one <= two) || two <= one { failures += 1 }
    if !(two >= two) || one >= two { failures += 1 }
    if !(one == Span(nanos: 1)) || one != Span(nanos: 1) { failures += 1 }

    if Light.red == Light.green || Light.amber != .amber { failures += 1 }

    if !(Loose(value: 1, note: "a") == Loose(value: 1, note: "b")) { failures += 1 }

    let q = Pair(a: p, b: two, name: "q", when: 0.5)
    if !(q == Pair(a: Point(x: 1, y: 2), b: Span(nanos: 2), name: "q", when: 0.5)) { failures += 1 }
    if q == Pair(a: p, b: two, name: "r", when: 0.5) { failures += 1 }

    print(failures == 0 ? "ok" : "failed \(failures)")
    return failures
}
