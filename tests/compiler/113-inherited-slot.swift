// A slot two levels above the class that fills it.
class A { func v() -> Int32 { return 42 } }
class B: A { func other() -> Int32 { return 1 } }
class C: B { }
func take(_ a: A) -> Int32 { return a.v() }
func main() -> Int32 { return take(C()) }
