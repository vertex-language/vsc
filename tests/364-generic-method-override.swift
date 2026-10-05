// An override of a generic method is reached through a superclass reference.
class Base {
    func describe<T>(_ x: T) -> String { "base \(x)" }
}
final class Sub: Base {
    override func describe<T>(_ x: T) -> String { "sub \(x)" }
}
let b: Base = Sub()
print(b.describe(3), b.describe("s"), Base().describe(1.5))
