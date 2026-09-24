// An initializer used as a function value.
struct Wrapper: CustomStringConvertible {
    let value: Int
    var description: String { "W\(value)" }
}
print([1, 2, 3].map(Wrapper.init))
print([1, 2, 3].map(String.init), ["4", "x"].map { Int($0) })
let make: (Int) -> Wrapper = Wrapper.init(value:)
print(make(9), [65, 66].map { Character(UnicodeScalar($0)!) })
