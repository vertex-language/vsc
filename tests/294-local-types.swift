// Types declared inside a function.
func f() -> Int {
    struct S { var a: Int; func twice() -> Int { a * 2 } }
    enum E { case x, y }
    let s = S(a: 4)
    let e = E.y
    return s.twice() + (e == .y ? 1 : 0)
}
print(f())
