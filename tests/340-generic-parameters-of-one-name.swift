// Two different type parameters named T -- a generic type's and its generic
// method's -- are different types.
struct Box<T> {
    let value: T
    func describe<T>(_ other: T) -> String {
        "\(type(of: value)) \(type(of: other))"
    }
    func same<T: Equatable>(_ a: T, _ b: T) -> Bool { a == b }
}
let b = Box(value: 1)
print(b.describe("s"), b.describe(2.5), b.same("x", "x"), Box(value: "v").describe(3))
