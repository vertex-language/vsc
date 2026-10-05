// A required initializer every subclass must have, used through a metatype.
class Widget {
    let id: Int
    required init(id: Int) { self.id = id }
    func kind() -> String { "widget" }
}
class Button: Widget {
    required init(id: Int) { super.init(id: id * 10) }
    override func kind() -> String { "button" }
}
func make(_ type: Widget.Type, _ id: Int) -> Widget { type.init(id: id) }
for t: Widget.Type in [Widget.self, Button.self] {
    let w = make(t, 3)
    print(w.kind(), w.id)
}
