// The check-form API markup lowers to (proposed_vsx.md §9.2), written
// here as the run tests' stand-in for ui/component and web/dom: it builds
// a tree, renders it to HTML, and keeps each element's handlers so a test
// can fire them. Every tests/vsx/run program is compiled with this file.
package main

// The events are declared at top level, under the names dom gives them:
// vsc does not yet cast to a class nested in an enum, and names a type
// reached through an alias in an enum by the alias's own name.
class Event {
    var Type: string = ""
    var DefaultPrevented = false
    init() {}
    func PreventDefault() { DefaultPrevented = true }
}
final class MouseEvent: Event {}
final class PointerEvent: Event {}
final class WheelEvent: Event {}
final class KeyboardEvent: Event {}
final class InputEvent: Event {}
final class SubmitEvent: Event {}
final class FocusEvent: Event {}
final class CompositionEvent: Event {}
final class DragEvent: Event {}
final class LayoutEvent: Event {}

enum dom {
    typealias Event = main.Event
    typealias MouseEvent = main.MouseEvent
    typealias PointerEvent = main.PointerEvent
    typealias WheelEvent = main.WheelEvent
    typealias KeyboardEvent = main.KeyboardEvent
    typealias InputEvent = main.InputEvent
    typealias SubmitEvent = main.SubmitEvent
    typealias FocusEvent = main.FocusEvent
    typealias CompositionEvent = main.CompositionEvent
    typealias DragEvent = main.DragEvent
    typealias LayoutEvent = main.LayoutEvent
}

func makeEvent(_ type: string) -> Event {
    var e: Event
    switch type {
    case "click", "dblclick", "contextmenu", "mousedown", "mouseup", "mousemove", "mouseenter", "mouseleave":
        e = MouseEvent()
    case "pointerdown", "pointerup", "pointermove": e = PointerEvent()
    case "wheel": e = WheelEvent()
    case "keydown", "keyup": e = KeyboardEvent()
    case "input": e = InputEvent()
    case "submit": e = SubmitEvent()
    case "focus", "blur": e = FocusEvent()
    case "compositionupdate": e = CompositionEvent()
    case "dragstart", "dragover", "drop": e = DragEvent()
    case "layout": e = LayoutEvent()
    default: e = Event()
    }
    e.Type = type
    return e
}

protocol Renderable {
    func Nodes() -> [Node]
}

final class Node: Renderable {
    var Tag: string = ""
    var Text: string = ""
    var Attrs: [(string, string)] = []
    var Classes: [string] = []
    var Styles: [(string, string)] = []
    var Handlers: [(string, (dom.Event) -> void)] = []
    var Children: [Node] = []
    init() {}
    func Nodes() -> [Node] { return Tag.isEmpty && Text.isEmpty ? Children : [self] }

    func Html() -> string {
        if Tag.isEmpty && !Text.isEmpty { return escape(Text) }
        if Tag.isEmpty { return Children.map { $0.Html() }.joined() }
        var s = "<" + Tag
        for (n, v) in Attrs { s += v == "\u{0}" ? " \(n)" : " \(n)=\"\(escape(v))\"" }
        if !Classes.isEmpty { s += " class=\"\(Classes.joined(separator: " "))\"" }
        if !Styles.isEmpty { s += " style=\"\(Styles.map { "\($0.0): \($0.1)" }.joined(separator: "; "))\"" }
        s += ">"
        for c in Children { s += c.Html() }
        return s + "</" + Tag + ">"
    }

    // Fire runs the first handler for an event, depth first.
    func Fire(_ type: string) -> bool {
        for (t, h) in Handlers where t == type {
            let e = Event()
            e.Type = type
            h(e)
            return true
        }
        for c in Children where c.Fire(type) { return true }
        return false
    }
}

func escape(_ s: string) -> string {
    var out = ""
    for ch in s {
        switch ch {
        case "&": out += "&amp;"
        case "<": out += "&lt;"
        case ">": out += "&gt;"
        case "\"": out += "&quot;"
        default: out.append(ch)
        }
    }
    return out
}

func textNode(_ s: string) -> Node {
    let n = Node()
    n.Text = s
    return n
}

extension string: Renderable { func Nodes() -> [Node] { return [textNode(self)] } }
extension int: Renderable { func Nodes() -> [Node] { return [textNode("\(self)")] } }
extension float64: Renderable { func Nodes() -> [Node] { return [textNode("\(self)")] } }
extension bool: Renderable { func Nodes() -> [Node] { return [] } }
extension Array: Renderable where Element: Renderable {
    func Nodes() -> [Node] { return self.flatMap { $0.Nodes() } }
}
extension Optional: Renderable where Wrapped: Renderable {
    func Nodes() -> [Node] {
        if let w = self { return w.Nodes() }
        return []
    }
}

typealias Children = () -> Node

enum component {
    struct Attribute {
        let apply: (Node) -> void

        static func Static(_ name: string, _ value: string) -> Attribute {
            return Attribute(apply: { n in
                if name == "class" { n.Classes.append(value) } else { n.Attrs.append((name, value)) }
            })
        }
        static func Value<V>(_ name: string, _ value: V) -> Attribute {
            return Attribute(apply: { n in
                if let b = value as? bool {
                    if b { n.Attrs.append((name, "\u{0}")) }
                } else if name == "class" {
                    n.Classes.append("\(value)")
                } else {
                    n.Attrs.append((name, "\(value)"))
                }
            })
        }
        static func On<E: dom.Event>(_ name: string, _ type: E.Type, _ handler: (E) -> void) -> Attribute {
            return Attribute(apply: { n in
                n.Handlers.append((name, { _ in
                    // vsc cannot yet call a required init through a
                    // generic metatype, so the event is made by name.
                    let e = makeEvent(name)
                    if let typed = e as? E { handler(typed) }
                }))
            })
        }
        static func Live<V>(_ name: string, _ f: () -> V) -> Attribute { return Value(name, f()) }
        static func LiveClass(_ name: string, _ f: () -> bool) -> Attribute { return Class(name, f()) }
        static func LiveStyle<V>(_ property: string, _ f: () -> V) -> Attribute { return Style(property, f()) }
        static func Class(_ name: string, _ on: bool) -> Attribute {
            return Attribute(apply: { n in if on { n.Classes.append(name) } })
        }
        static func Style<V>(_ property: string, _ value: V) -> Attribute {
            return Attribute(apply: { n in n.Styles.append((property, "\(value)")) })
        }
        static func Ref<V>(_ ref: V) -> Attribute {
            return Attribute(apply: { _ in })
        }
        static func Spread(_ attrs: [string: string]) -> Attribute {
            return Attribute(apply: { n in
                for k in attrs.keys.sorted() { n.Attrs.append((k, attrs[k]!)) }
            })
        }
    }

    static func Element(_ tag: string, _ attributes: [Attribute], _ children: [any Renderable]) -> Node {
        let n = Node()
        n.Tag = tag
        for a in attributes { a.apply(n) }
        for c in children { n.Children += c.Nodes() }
        return n
    }

    // The emit form's live parts, taken at once: this prelude renders once.
    static func Live<V: Renderable>(_ f: () -> V) -> Node { return Fragment([f()]) }
    static func Component<T>(_ f: () -> T) -> T { return f() }

    static func Fragment(_ children: [any Renderable]) -> Node {
        let n = Node()
        for c in children { n.Children += c.Nodes() }
        return n
    }
}

// For is the keyed list (proposed_vsx.md §5.4). Here it renders each item
// once; keys are checked for being given the right type and not used.
func For<T>(each: [T], key: ((T) -> int)? = nil, children: (T) -> Node) -> Node {
    let n = Node()
    for item in each { n.Children += children(item).Nodes() }
    return n
}
