// Two functions' types of one name are two types: each its own methods,
// and its own conformance, beside a module-level type of the name too.
protocol P { func v() -> Int }
func f() -> Int {
    struct S: P { var a: Int; func v() -> Int { a } }
    let p: any P = S(a: 1)
    return p.v() + S(a: 10).v()
}
func g() -> Int {
    struct S: P { var a: Int; func v() -> Int { a * 100 } }
    let p: any P = S(a: 2)
    return p.v()
}
struct S { var c = 3 }
print(f(), g(), S().c)
