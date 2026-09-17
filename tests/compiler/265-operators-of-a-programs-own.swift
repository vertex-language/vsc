// Operators a program declares: static on the type they work on, which
// is the usual way, and at the top level, which is the other. The operand
// types choose among overloads, a literal operand takes the type the
// declaration wants, and a leading-dot operand is read as the parameter's
// type rather than the other operand's.
struct Span {
    let nanos: Int64
    static func seconds(_ n: Int64) -> Span { return Span(nanos: n * 1_000_000_000) }
    static func + (a: Span, b: Span) -> Span { return Span(nanos: a.nanos + b.nanos) }
    static func * (a: Span, n: Int64) -> Span { return Span(nanos: a.nanos * n) }
    static prefix func - (a: Span) -> Span { return Span(nanos: -a.nanos) }
    static func < (a: Span, b: Span) -> Bool { return a.nanos < b.nanos }
    static func == (a: Span, b: Span) -> Bool { return a.nanos == b.nanos }
}

struct Moment {
    let nanos: Int64
    static func + (m: Moment, s: Span) -> Moment { return Moment(nanos: m.nanos + s.nanos) }
    static func - (m: Moment, s: Span) -> Moment { return Moment(nanos: m.nanos - s.nanos) }
    static func - (a: Moment, b: Moment) -> Span { return Span(nanos: a.nanos - b.nanos) }
}

struct Meters { let n: Int }
func + (a: Meters, b: Meters) -> Meters { return Meters(n: a.n + b.n) }
prefix func ! (a: Meters) -> Bool { return a.n == 0 }

func main() -> Int32 {
    var failures: Int32 = 0
    let a = Span.seconds(2)
    let b = Span(nanos: 5)

    if (a + b).nanos != 2_000_000_005 { failures += 1 }
    if (b * 3).nanos != 15 { failures += 1 }
    if (-b).nanos != -5 { failures += 1 }
    if !(b < a) { failures += 1 }
    if !(a == Span.seconds(2)) { failures += 1 }

    // Two `-`s on Moment, told apart by the right operand's type.
    let m = Moment(nanos: 100)
    let earlier = m - Span(nanos: 40)
    let gap: Span = m - earlier
    if earlier.nanos != 60 || gap.nanos != 40 { failures += 1 }

    // A leading-dot operand is a Span, which is what `+` on Moment takes.
    if (m + .seconds(1)).nanos != 1_000_000_100 { failures += 1 }
    if !(a == .seconds(2)) { failures += 1 }

    // Top-level operators, beside core's: Int's + still means Int's.
    if (Meters(n: 2) + Meters(n: 3)).n != 5 { failures += 1 }
    if !(!Meters(n: 0)) { failures += 1 }
    let sum = 2 + 3
    if sum != 5 { failures += 1 }

    print(failures == 0 ? "ok" : "failed \(failures)")
    return failures
}
