// is and as? to classes nested in an enum, and a class reached through a
// typealias declared in the enum.
enum UI {
    class View { func name() -> String { "view" } }
    final class Button: View { override func name() -> String { "button" } }
    typealias Alias = Button
}
let v: Any = UI.Button()
print(v is UI.Button, v is UI.View, (v as? UI.View)?.name() ?? "nil")
let a = UI.Alias()
print(a.name(), a is UI.Button)
let o: AnyObject = a
print(o is UI.Alias, (o as? UI.View)?.name() ?? "nil")
let plain: Any = UI.View()
print(plain is UI.Button)
