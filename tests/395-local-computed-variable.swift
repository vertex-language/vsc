// A computed variable declared in a function reads its getter each time,
// and writes through its setter.
final class Store { var v = 0 }
func run() {
    let s = Store()
    var x: Int {
        get { s.v }
        set { s.v = newValue }
    }
    print(x)
    s.v = 5
    print(x)
    x = 7
    print(s.v, x)
}
run()
final class Box<T> {
    var v: T
    init(_ v: T) { self.v = v }
}
func peek<T>(_ b: Box<T>, _ next: T) {
    var x: T { b.v }
    print(x)
    b.v = next
    print(x)
}
peek(Box(1), 2)
peek(Box("a"), "b")
