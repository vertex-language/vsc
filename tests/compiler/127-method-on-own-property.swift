// A method called on a reference the receiver holds.
final class Inner { func v() -> Int32 { return 42 } }
final class Outer { var i = Inner(); func run() -> Int32 { return i.v() } }
func main() -> Int32 { return Outer().run() }
