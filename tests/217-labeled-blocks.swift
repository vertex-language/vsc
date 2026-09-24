// A label on if or do lets break leave the block early.
func check(_ n: Int) -> String {
    var s = "start"
    validation: do {
        if n < 0 { s += " negative"; break validation }
        if n > 100 { s += " large"; break validation }
        s += " ok"
    }
    test: if n % 2 == 0 {
        if n == 0 { break test }
        s += " even"
    }
    return s + " end"
}
for n in [-1, 0, 42, 7, 500] { print(check(n)) }
