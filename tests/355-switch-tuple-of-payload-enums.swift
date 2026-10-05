// switch over a tuple of two payload enums whose payloads hold references.
final class Obj { let id: Int; init(_ id: Int) { self.id = id } }
enum Value { case number(Double), string(String), object(Obj), undefined }
func looseEquals(_ a: Value, _ b: Value) -> Bool {
    switch (a, b) {
    case (.number(let x), .number(let y)): return x == y
    case (.string(let x), .string(let y)): return x == y
    case (.object(let x), .object(let y)): return x === y
    case (.undefined, .undefined): return true
    default: return false
    }
}
let o = Obj(1)
print(looseEquals(.number(1), .number(1)), looseEquals(.string("a"), .string("b")),
      looseEquals(.object(o), .object(o)), looseEquals(.object(o), .object(Obj(1))),
      looseEquals(.undefined, .number(0)))
