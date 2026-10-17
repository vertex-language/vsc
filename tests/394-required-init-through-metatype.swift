// init() called through a generic metatype E.Type, for a class whose
// init() is required.
class Element {
    required init() {}
    func tag() -> String { "element" }
}
final class Div: Element { override func tag() -> String { "div" } }
func make<E: Element>(_ t: E.Type) -> E { t.init() }
print(make(Div.self).tag(), make(Element.self).tag())
func makeAll<E: Element>(_ t: E.Type, _ n: Int) -> [String] { (0..<n).map { _ in t.init().tag() } }
print(makeAll(Div.self, 2))
