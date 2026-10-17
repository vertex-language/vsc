// A function returns a tuple of two existentials.
protocol Port {
    func send(_ s: String) -> String
}
final class End: Port {
    let name: String
    init(_ name: String) { self.name = name }
    deinit { print(name, "freed") }
    func send(_ s: String) -> String { name + ":" + s }
}
struct Loop: Port {
    let a: Int, b: Int, c: Int, d: Int, e: Int
    func send(_ s: String) -> String { s + String(a + b + c + d + e) }
}
func pipe() -> (any Port, any Port) { (End("left"), End("right")) }
func mixed() -> (any Port, any Port) { (Loop(a: 1, b: 2, c: 3, d: 4, e: 5), End("solo")) }
do {
    let (l, r) = pipe()
    print(l.send("a"), r.send("b"))
    let m = mixed()
    print(m.0.send("n"), m.1.send("c"))
}
print("done")
