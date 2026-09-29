// A top-level let is a global: it is never destroyed, even at exit.
protocol P { func hello() }
final class C: P {
    func hello() { print("hello") }
    deinit { print("deinit") }
}
let e: any P = C()
e.hello()
print("end")
