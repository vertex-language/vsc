// Assigning a computed property with a nonmutating set through a let.
final class Storage { var text = "" }
struct Element {
    let storage: Storage
    var textContent: String {
        get { storage.text }
        nonmutating set { storage.text = newValue }
    }
}
let p = Element(storage: Storage())
p.textContent = "hello"
print(p.textContent)
