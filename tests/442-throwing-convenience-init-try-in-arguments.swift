// A throwing convenience init that tries in its self.init arguments.
struct Bad: Error {}
func readText(_ path: String) throws -> String {
    if path.isEmpty { throw Bad() }
    return "recipe:" + path
}
func decode(_ s: String) throws -> Int {
    guard s.hasPrefix("recipe:") else { throw Bad() }
    return s.count
}
final class Field {
    let size: Int
    init(_ size: Int) { self.size = size }
    convenience init(recipe path: String) throws {
        self.init(try decode(try readText(path)))
    }
}
print(try Field(recipe: "hills").size)
print((try? Field(recipe: "")) == nil)
