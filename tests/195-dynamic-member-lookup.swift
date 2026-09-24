// @dynamicMemberLookup turns member syntax into subscript calls.
@dynamicMemberLookup
struct Settings {
    var values: [String: Int]
    subscript(dynamicMember key: String) -> Int {
        get { values[key] ?? 0 }
        set { values[key] = newValue }
    }
}
@dynamicMemberLookup
struct Wrapper<T> {
    var inner: T
    subscript<U>(dynamicMember path: KeyPath<T, U>) -> U { inner[keyPath: path] }
}
var s = Settings(values: ["width": 80])
s.height = 24
print(s.width, s.height, s.depth)
let w = Wrapper(inner: "hello")
print(w.count, w.first as Any)
