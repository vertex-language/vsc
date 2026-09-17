// A struct passed to the same function twice. The second call must
// not restate the callee's type: a declaration is filled in at the
// first call that names it, and doing it again gave the function its
// parameters twice.
struct Pair { var a: Int32; var b: Int32 }

func sumPair(_ p: Pair) -> Int32 { return p.a + p.b }
func bothWays(_ p: Pair, _ q: Pair) -> Int32 { return sumPair(p) + sumPair(q) }

func main() -> Int32 {
    let p = Pair(a: 20, b: 15)
    if sumPair(p) != 35 { return 91 }
    if sumPair(p) != 35 { return 92 }
    if bothWays(p, p) != 70 { return 93 }
    return sumPair(p) + sumPair(p) - 28
}
