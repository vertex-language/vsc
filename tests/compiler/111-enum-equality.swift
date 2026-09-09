// Two enum values are equal when their cases are. Swift synthesizes
// that comparison; there is no library here to declare it, so it is
// emitted where the call would have been -- and the tag is the value,
// so the operands go to the comparison as they are.
enum Big { case a, b, c, d, e, f, g, h }
enum Two { case x, y }

func same(_ a: Big, _ b: Big) -> Bool { return a == b }

func main() -> Int32 {
    let p = Big.f
    let q = Big.f
    let r = Big.c
    var score: Int32 = 0
    if p == q { score = score + 1 }
    if p != r { score = score + 2 }
    if !(p == r) { score = score + 4 }
    if Two.x == Two.x { score = score + 8 }
    if Two.x != Two.y { score = score + 16 }

    // Through a call, so the values are not constants the compiler
    // can fold the comparison away on.
    if same(p, q) { score = score + 32 }
    if !same(p, r) { score = score + 64 }
    return score - 85
}
