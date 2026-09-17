// A slot inherited two levels down finds the nearest body above it.
class A {
    func one() -> Int32 { return 1 }
    func two() -> Int32 { return 2 }
}

class B: A {
    override func one() -> Int32 { return 10 }
}

class C: B {
    override func two() -> Int32 { return 20 }
}

func sum(_ a: A) -> Int32 { return a.one() + a.two() }

func main() -> Int32 {
    return sum(A()) + sum(B()) + sum(C())
}
