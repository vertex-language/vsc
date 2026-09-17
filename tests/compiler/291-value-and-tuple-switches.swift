// A switch over a value that is not an enum matches it without taking it:
// a String against literals and a bound name with a where clause, and a
// tuple element by element -- literals, ranges, names, `_`, and a let
// written outside the parentheses. The value lives through the switch and
// is let go of once, whichever arm runs.

func f(_ s: String) -> Int {
    switch s {
    case "a": return 1
    case let k where k.count > 3: return k.count
    default: return 0
    }
}
func classify(_ p: (Int, String)) -> String {
    switch p {
    case (0, _): return "zero"
    case (1, let s): return "one " + s
    case let (n, "x"): return "x at " + String(n)
    case (let n, let s) where n > 100: return "big " + s
    case (2...5, _): return "small"
    default: return "other"
    }
}
func grid(_ x: Int, _ y: Int) -> Int {
    switch (x, y) {
    case (0, 0): return 0
    case (_, 0): return 1
    case (0, _): return 2
    case let (a, b) where a == b: return 3
    default: return 4
    }
}
func main() -> Int32 {
    var out = ""
    var total = 0
    for i in 0..<300 {
        out = classify((i % 7, "s" + String(i))) + classify((i, "x")) + classify((150, "b"))
        total += grid(i % 3, i % 2) + f("a") + f("hello" + String(i)) + f("zz")
    }
    print(classify((0, "a")), classify((1, "a")), classify((9, "x")), classify((200, "q")), classify((3, "q")), classify((7, "q")), out, total)
    return Int32(total % 256)
}
