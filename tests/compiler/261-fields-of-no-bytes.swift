// Values with fields that hold nothing: a one-case enum, an empty struct.
//
// Such a field has a type and no bytes, and Swift leaves it out of how the
// value travels -- a `(kind: Kind, window: UInt64, view: UInt64)` whose Kind
// has one case is two registers, not three. The field is still there to
// read, match on, and pass along; it just occupies nothing.
enum Kind { case appKit }
struct Marker {}
enum Two { case a, b }

func parts() -> (kind: Kind, window: UInt64, view: UInt64) {
    return (kind: .appKit, window: 7, view: 9)
}

struct Tagged {
    var before: Marker
    var value: Int32
    var kind: Kind
    var other: Int32
    var after: Marker
}

func make(_ n: Int32) -> Tagged {
    return Tagged(before: Marker(), value: n, kind: .appKit, other: n * 2, after: Marker())
}

func take(_ t: Tagged) -> Int32 {
    switch t.kind {
    case .appKit: return t.value + t.other
    }
}

// Wide enough to go by address, with a field of no bytes in the middle.
struct Wide {
    var a: Int64
    var none: Marker
    var b: Int64
    var c: Int64
    var kind: Kind
    var d: Int64
    var e: Int64
}

func sum(_ w: Wide) -> Int64 {
    return w.a + w.b + w.c + w.d + w.e
}

func pair(_ k: Kind, _ n: Int32, _ m: Marker) -> (Kind, Int32, Two) {
    return (k, n + 1, .b)
}

func main() -> Int32 {
    var total: Int32 = 0
    let p = parts()
    switch p.kind {
    case .appKit: total += Int32(p.window + p.view)          // 16
    }
    let t = make(5)
    total += take(t)                                          // 15
    let w = Wide(a: 1, none: Marker(), b: 2, c: 3, kind: .appKit, d: 4, e: 5)
    total += Int32(sum(w))                                    // 15
    let (k, n, two) = pair(.appKit, 40, Marker())
    switch k {
    case .appKit: total += n                                  // 41
    }
    if two == .b { total += 100 }
    return total % 251
}
