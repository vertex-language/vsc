// One case of several tuple patterns that bind the same name.
final class Obj { let id: Int; init(_ id: Int) { self.id = id } }
enum Value { case object(Obj), undefined, null, number(Int) }
func pick(_ a: Value, _ b: Value) -> String {
    switch (a, b) {
    case (.object(let o), .undefined), (.object(let o), .null):
        return "object \(o.id) with nothing"
    case (.number(let n), _), (_, .number(let n)):
        return "number \(n)"
    default:
        return "other"
    }
}
print(pick(.object(Obj(3)), .undefined), pick(.object(Obj(4)), .null), pick(.null, .number(9)), pick(.null, .null))
