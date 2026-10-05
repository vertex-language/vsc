// A temporary lives to the end of the full expression it is made in, not to
// the end of the scope.
final class C {
    let n: Int
    init(_ n: Int) { self.n = n }
    deinit { print("deinit", n) }
}
func f(_ c: C) -> Int { c.n + 1 }
func run() {
    print(f(C(1)))
    print("after first")
    let x = f(C(2)) * 10
    print("after second", x)
}
run()
print("end")
