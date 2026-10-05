// A subclass method loops straight over an inherited computed property.
class Base {
    var storage = ["b", "a", "c"]
    var keys: [String] { storage.sorted() }
}
final class Derived: Base {
    func joined() -> String {
        var out = ""
        for k in keys { out += k }
        return out
    }
}
print(Derived().joined())
