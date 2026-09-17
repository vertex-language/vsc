// An instance of a subclass holds its superclass's stored
// properties, initialized from where they were declared, and reached
// by their bare names as well as through self.
class A {
    var n: Int32 = 1
    var m: Int32 = 2
    func readA() -> Int32 { return n * 10 + m }
}
class B: A {
    var k: Int32 = 4
    func readB() -> Int32 { return self.n * 100 + m * 10 + k }
}
class C: B {
    var j: Int32 = 8
    func readC() -> Int32 { return n + m + k + j }
    override func readA() -> Int32 { return j }
}
func through(_ a: A) -> Int32 { return a.readA() }
func main() -> Int32 {
    let b = B()
    b.n = 7
    let c = C()
    c.m = 3
    return (b.readB() + c.readC() + through(A()) + through(c) + b.readA()) % 128
}
