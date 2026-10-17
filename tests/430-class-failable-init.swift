// A class with a failable initializer: nil before and after its
// properties are set, and an instance when it succeeds.
final class Render {
    let handle: Int
    let name: String
    init?(_ handle: Int, name: String) {
        if handle < 0 { return nil }
        self.handle = handle
        self.name = name
        if name.isEmpty { return nil }
    }
    deinit { print("render", handle, "freed") }
}
print(Render(-1, name: "x") == nil)
print(Render(2, name: "") == nil)
if let r = Render(3, name: "metal") { print(r.handle, r.name) }
print("done")
