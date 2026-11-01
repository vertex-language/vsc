// Built-in .vsx v2 prelude: UI types, DOM events/elements, hooks, and
// element/component lowering targets available in every .vsx compilation unit.

class HTMLElement {
    var tagName: string = ""
    var id: string = ""
    var className: string = ""
    var textContent: string = ""
    var innerHTML: string = ""
    var hidden: bool = false
    init() {}
    func focus() {}
    func blur() {}
    func click() {}
}

class HTMLDivElement: HTMLElement {}
class HTMLSpanElement: HTMLElement {}

class HTMLButtonElement: HTMLElement {
    var disabled: bool = false
    var type: string = "button"
    var value: string = ""
}

class HTMLInputElement: HTMLElement {
    var value: string = ""
    var checked: bool = false
    var disabled: bool = false
    var type: string = "text"
    var name: string = ""
    var placeholder: string = ""
    func select() {}
}

class HTMLTextAreaElement: HTMLElement {
    var value: string = ""
    var disabled: bool = false
    var rows: int = 2
    var cols: int = 20
    var placeholder: string = ""
}

class HTMLSelectElement: HTMLElement {
    var value: string = ""
    var selectedIndex: int = 0
    var disabled: bool = false
}

class HTMLFormElement: HTMLElement {
    var action: string = ""
    var method: string = "get"
    func submit() {}
    func reset() {}
}

class HTMLAnchorElement: HTMLElement {
    var href: string = ""
    var target: string = ""
}

class HTMLImageElement: HTMLElement {
    var src: string = ""
    var alt: string = ""
    var width: int = 0
    var height: int = 0
}

class HTMLCanvasElement: HTMLElement {
    var width: int = 300
    var height: int = 150
}

class FormData {
    var entries: [string: string] = [:]
    init() {}
    func get(_ name: string) -> string? { return entries[name] }
    func set(_ name: string, _ value: string) { entries[name] = value }
    func has(_ name: string) -> bool { return entries[name] != nil }
}

class Event {
    var Type: string = ""
    var type: string { return Type }
    var DefaultPrevented: bool = false
    var defaultPrevented: bool { return DefaultPrevented }
    var PropagationStopped: bool = false
    var propagationStopped: bool { return PropagationStopped }
    var target: HTMLElement = HTMLElement()
    var currentTarget: HTMLElement = HTMLElement()
    init() {}
    func PreventDefault() { DefaultPrevented = true }
    func preventDefault() { DefaultPrevented = true }
    func StopPropagation() { PropagationStopped = true }
    func stopPropagation() { PropagationStopped = true }
}

final class MouseEvent: Event {
    var clientX: float64 = 0
    var clientY: float64 = 0
    var pageX: float64 = 0
    var pageY: float64 = 0
    var button: int = 0
    var altKey: bool = false
    var ctrlKey: bool = false
    var metaKey: bool = false
    var shiftKey: bool = false
}

final class PointerEvent: Event {
    var pointerId: int = 0
    var clientX: float64 = 0
    var clientY: float64 = 0
    var pressure: float64 = 0
}

final class WheelEvent: Event {
    var deltaX: float64 = 0
    var deltaY: float64 = 0
}

final class KeyboardEvent: Event {
    var key: string = ""
    var code: string = ""
    var altKey: bool = false
    var ctrlKey: bool = false
    var metaKey: bool = false
    var shiftKey: bool = false
}

final class InputEvent: Event {
    var data: string = ""
}

final class ChangeEvent: Event {
    var value: string = ""
}

final class SubmitEvent: Event {}
final class FocusEvent: Event {}
final class CompositionEvent: Event {
    var data: string = ""
}
final class DragEvent: Event {}
final class LayoutEvent: Event {
    var width: float64 = 0
    var height: float64 = 0
}

func _vsxMakeEvent(_ type: string) -> Event {
    var e: Event
    switch type {
    case "click", "dblclick", "contextmenu", "mousedown", "mouseup", "mousemove", "mouseenter", "mouseleave":
        e = MouseEvent()
    case "pointerdown", "pointerup", "pointermove":
        e = PointerEvent()
    case "wheel":
        e = WheelEvent()
    case "keydown", "keyup":
        e = KeyboardEvent()
    case "input", "beforeinput":
        e = InputEvent()
    case "change":
        e = ChangeEvent()
    case "submit":
        e = SubmitEvent()
    case "focus", "blur":
        e = FocusEvent()
    case "compositionupdate", "compositionstart", "compositionend":
        e = CompositionEvent()
    case "dragstart", "dragover", "drop", "dragenter", "dragleave", "dragend":
        e = DragEvent()
    case "layout":
        e = LayoutEvent()
    default:
        e = Event()
    }
    e.Type = type
    return e
}

protocol Renderable {
    func Nodes() -> [Node]
}

final class Node: Renderable, ExpressibleByNilLiteral, ExpressibleByStringLiteral, ExpressibleByIntegerLiteral, ExpressibleByFloatLiteral, ExpressibleByBooleanLiteral {
    var Tag: string = ""
    var Text: string = ""
    var Key: string = ""
    var Attrs: [(string, string)] = []
    var Classes: [string] = []
    var Styles: [(string, string)] = []
    var Handlers: [(string, (dom.Event) -> void)] = []
    var Children: [Node] = []

    init() {}
    convenience init(nilLiteral: ()) {
        self.init()
    }
    convenience init(stringLiteral value: string) {
        self.init()
        self.Text = value
    }
    convenience init(integerLiteral value: int) {
        self.init()
        self.Text = "\(value)"
    }
    convenience init(floatLiteral value: float64) {
        self.init()
        self.Text = "\(value)"
    }
    convenience init(booleanLiteral value: bool) {
        self.init()
    }

    func syncFrom(_ other: Node) {
        self.Tag = other.Tag
        self.Text = other.Text
        self.Key = other.Key
        self.Attrs = other.Attrs
        self.Classes = other.Classes
        self.Styles = other.Styles
        self.Handlers = other.Handlers
        self.Children = other.Children
    }

    func setAttr(_ name: string, _ val: string) {
        for i in 0..<Attrs.count {
            if Attrs[i].0 == name {
                Attrs[i] = (name, val)
                return
            }
        }
        Attrs.append((name, val))
    }

    func removeAttr(_ name: string) {
        var next: [(string, string)] = []
        for a in Attrs {
            if a.0 != name { next.append(a) }
        }
        Attrs = next
    }

    func setStyle(_ prop: string, _ val: string) {
        for i in 0..<Styles.count {
            if Styles[i].0 == prop {
                Styles[i] = (prop, val)
                return
            }
        }
        Styles.append((prop, val))
    }

    func Nodes() -> [Node] {
        return Tag.isEmpty && Text.isEmpty ? Children : [self]
    }

    func Html() -> string {
        if Tag.isEmpty && !Text.isEmpty { return _vsxEscape(Text) }
        if Tag.isEmpty { return Children.map { $0.Html() }.joined() }
        var s = "<" + Tag
        for (n, v) in Attrs {
            s += v == "\u{0}" ? " \(n)" : " \(n)=\"\(_vsxEscape(v))\""
        }
        if !Classes.isEmpty {
            s += " class=\"\(Classes.joined(separator: " "))\""
        }
        if !Styles.isEmpty {
            s += " style=\"\(Styles.map { "\($0.0): \($0.1)" }.joined(separator: "; "))\""
        }
        s += ">"
        for c in Children { s += c.Html() }
        return s + "</" + Tag + ">"
    }

    func html() -> string { return Html() }

    func Fire(_ type: string) -> bool {
        for (t, h) in Handlers where t == type {
            let e = _vsxMakeEvent(type)
            h(e)
            return true
        }
        for c in Children where c.Fire(type) { return true }
        return false
    }

    func fire(_ type: string) -> bool { return Fire(type) }
}

typealias Element = Node
typealias Children = () -> Node

func renderToString(_ node: Node) -> string {
    return node.Html()
}

func _vsxEscape(_ s: string) -> string {
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

func _vsxTextNode(_ s: string) -> Node {
    let n = Node()
    n.Text = s
    return n
}

extension string: Renderable { func Nodes() -> [Node] { return [_vsxTextNode(self)] } }
extension int: Renderable { func Nodes() -> [Node] { return [_vsxTextNode("\(self)")] } }
extension float64: Renderable { func Nodes() -> [Node] { return [_vsxTextNode("\(self)")] } }
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

struct CSSValue: ExpressibleByStringLiteral, ExpressibleByIntegerLiteral, ExpressibleByFloatLiteral, ExpressibleByNilLiteral {
    var raw: string = ""
    init(stringLiteral value: string) { self.raw = value }
    init(integerLiteral value: int) { self.raw = "\(value)px" }
    init(floatLiteral value: float64) { self.raw = "\(value)" }
    init(nilLiteral: ()) { self.raw = "" }
}

struct Style {
    var color: CSSValue = ""
    var backgroundColor: CSSValue = ""
    var background: CSSValue = ""
    var display: CSSValue = ""
    var position: CSSValue = ""
    var top: CSSValue = ""
    var right: CSSValue = ""
    var bottom: CSSValue = ""
    var left: CSSValue = ""
    var width: CSSValue = ""
    var height: CSSValue = ""
    var minWidth: CSSValue = ""
    var minHeight: CSSValue = ""
    var maxWidth: CSSValue = ""
    var maxHeight: CSSValue = ""
    var margin: CSSValue = ""
    var marginTop: CSSValue = ""
    var marginRight: CSSValue = ""
    var marginBottom: CSSValue = ""
    var marginLeft: CSSValue = ""
    var padding: CSSValue = ""
    var paddingTop: CSSValue = ""
    var paddingRight: CSSValue = ""
    var paddingBottom: CSSValue = ""
    var paddingLeft: CSSValue = ""
    var flex: CSSValue = ""
    var flexDirection: CSSValue = ""
    var alignItems: CSSValue = ""
    var justifyContent: CSSValue = ""
    var gap: CSSValue = ""
    var border: CSSValue = ""
    var borderRadius: CSSValue = ""
    var borderColor: CSSValue = ""
    var fontSize: CSSValue = ""
    var fontWeight: CSSValue = ""
    var lineHeight: CSSValue = ""
    var textAlign: CSSValue = ""
    var textDecoration: CSSValue = ""
    var opacity: CSSValue = ""
    var overflow: CSSValue = ""
    var cursor: CSSValue = ""
    var zIndex: CSSValue = ""
    var transform: CSSValue = ""
    var transition: CSSValue = ""
    var boxShadow: CSSValue = ""

    func pairs() -> [(string, string)] {
        var out: [(string, string)] = []
        func add(_ k: string, _ v: CSSValue) {
            if !v.raw.isEmpty { out.append((k, v.raw)) }
        }
        add("color", color)
        add("background-color", backgroundColor)
        add("background", background)
        add("display", display)
        add("position", position)
        add("top", top)
        add("right", right)
        add("bottom", bottom)
        add("left", left)
        add("width", width)
        add("height", height)
        add("min-width", minWidth)
        add("min-height", minHeight)
        add("max-width", maxWidth)
        add("max-height", maxHeight)
        add("margin", margin)
        add("margin-top", marginTop)
        add("margin-right", marginRight)
        add("margin-bottom", marginBottom)
        add("margin-left", marginLeft)
        add("padding", padding)
        add("padding-top", paddingTop)
        add("padding-right", paddingRight)
        add("padding-bottom", paddingBottom)
        add("padding-left", paddingLeft)
        add("flex", flex)
        add("flex-direction", flexDirection)
        add("align-items", alignItems)
        add("justify-content", justifyContent)
        add("gap", gap)
        add("border", border)
        add("border-radius", borderRadius)
        add("border-color", borderColor)
        add("font-size", fontSize)
        add("font-weight", fontWeight)
        add("line-height", lineHeight)
        add("text-align", textAlign)
        add("text-decoration", textDecoration)
        add("opacity", opacity)
        add("overflow", overflow)
        add("cursor", cursor)
        add("z-index", zIndex)
        add("transform", transform)
        add("transition", transition)
        add("box-shadow", boxShadow)
        return out
    }
}

protocol Props {}
struct EmptyProps: Props { init() {} }
struct EmptyState { init() {} }

struct HTMLAttributes<T>: Props {
    var id: string = ""
    var className: string = ""
    var title: string = ""
    var role: string = ""
    var tabIndex: int = 0
    var hidden: bool = false
    var style: Style = Style()
}

final class Ref<T> {
    var current: T
    init(_ current: T) {
        self.current = current
    }
}

class _HookSlot {
    init() {}
}

final class _HookCell<T>: _HookSlot {
    var value: T
    init(_ value: T) {
        self.value = value
        super.init()
    }
}

final class _Fiber {
    var hooks: [_HookSlot] = []
    var index: int = 0
    var renderFn: () -> Node
    var host: Node
    var mounted: bool = false
    var effects: [() -> void] = []

    init(renderFn: @escaping () -> Node, host: Node) {
        self.renderFn = renderFn
        self.host = host
    }

    func run() {
        let prev = _currentFiber
        _currentFiber = self
        self.index = 0
        self.effects = []
        let out = self.renderFn()
        _currentFiber = prev
        self.host.syncFrom(out)
        self.mounted = true
        for eff in self.effects {
            eff()
        }
    }
}

var _currentFiber: _Fiber? = nil
var _vsxIdCounter: int = 0

func useState<T>(_ initial: T) -> (T, (T) -> void) {
    if let fiber = _currentFiber {
        let idx = fiber.index
        fiber.index += 1
        if idx >= fiber.hooks.count {
            fiber.hooks.append(_HookCell<T>(initial))
        }
        if let cell = fiber.hooks[idx] as? _HookCell<T> {
            return (cell.value, { next in
                cell.value = next
                fiber.run()
            })
        }
    }
    let cell = _HookCell<T>(initial)
    return (cell.value, { next in cell.value = next })
}

func useState<T>(_ initial: () -> T) -> (T, (T) -> void) {
    return useState(initial())
}

func useReducer<S, A>(_ reducer: @escaping (S, A) -> S, _ initialState: S) -> (S, (A) -> void) {
    if let fiber = _currentFiber {
        let idx = fiber.index
        fiber.index += 1
        if idx >= fiber.hooks.count {
            fiber.hooks.append(_HookCell<S>(initialState))
        }
        if let cell = fiber.hooks[idx] as? _HookCell<S> {
            return (cell.value, { action in
                cell.value = reducer(cell.value, action)
                fiber.run()
            })
        }
    }
    let cell = _HookCell<S>(initialState)
    return (cell.value, { action in cell.value = reducer(cell.value, action) })
}

func useRef<T>(_ initial: T) -> Ref<T> {
    if let fiber = _currentFiber {
        let idx = fiber.index
        fiber.index += 1
        if idx >= fiber.hooks.count {
            fiber.hooks.append(_HookCell<Ref<T>>(Ref<T>(initial)))
        }
        if let cell = fiber.hooks[idx] as? _HookCell<Ref<T>> {
            return cell.value
        }
    }
    return Ref<T>(initial)
}

func useEffect(_ effect: @escaping () -> void, _ deps: [any Equatable] = []) {
    if let fiber = _currentFiber {
        if !fiber.mounted {
            fiber.effects.append(effect)
        }
    } else {
        effect()
    }
}

func useLayoutEffect(_ effect: @escaping () -> void, _ deps: [any Equatable] = []) {
    useEffect(effect, deps)
}

func useInsertionEffect(_ effect: @escaping () -> void, _ deps: [any Equatable] = []) {
    useEffect(effect, deps)
}

func useMemo<T>(_ compute: () -> T, _ deps: [any Equatable] = []) -> T {
    return compute()
}

func useCallback<F>(_ callback: F, _ deps: [any Equatable] = []) -> F {
    return callback
}

func useImperativeHandle<T>(_ ref: Ref<T?>, _ create: () -> T, _ deps: [any Equatable] = []) {
    ref.current = create()
}

func useId() -> string {
    if let fiber = _currentFiber {
        let idx = fiber.index
        fiber.index += 1
        if idx >= fiber.hooks.count {
            _vsxIdCounter += 1
            fiber.hooks.append(_HookCell<string>(":r\(_vsxIdCounter):"))
        }
        if let cell = fiber.hooks[idx] as? _HookCell<string> {
            return cell.value
        }
    }
    _vsxIdCounter += 1
    return ":r\(_vsxIdCounter):"
}

func useTransition() -> (bool, (@escaping () -> void) -> void) {
    return (false, { scope in scope() })
}

func startTransition(_ scope: () -> void) {
    scope()
}

func useDeferredValue<T>(_ value: T) -> T {
    return value
}

func useSyncExternalStore<T>(_ subscribe: (@escaping () -> void) -> (() -> void), _ getSnapshot: () -> T) -> T {
    return getSnapshot()
}

func useOptimistic<T>(_ passthrough: T) -> (T, (T) -> void) {
    return useState(passthrough)
}

func useOptimistic<S, A>(_ passthrough: S, _ reducer: @escaping (S, A) -> S) -> (S, (A) -> void) {
    return useReducer(reducer, passthrough)
}

func useActionState<S, P>(_ action: @escaping (S, P) -> S, _ initialState: S) -> (S, (P) -> void, bool) {
    let (state, dispatch) = useReducer(action, initialState)
    return (state, dispatch, false)
}

struct FormStatus {
    var pending: bool = false
    var data: FormData? = nil
    var method: string? = nil
    var action: string? = nil
    init(pending: bool = false) {
        self.pending = pending
    }
}

func useFormStatus() -> FormStatus {
    return FormStatus()
}

final class Context<T> {
    let defaultValue: T
    var _current: T
    init(_ defaultValue: T) {
        self.defaultValue = defaultValue
        self._current = defaultValue
    }
    func Provider(value: T, children: () -> Node) -> Node {
        self._current = value
        return component.Component({
            let prev = self._current
            self._current = value
            let out = children()
            self._current = prev
            return out
        })
    }
}

func createContext<T>(_ defaultValue: T) -> Context<T> {
    return Context<T>(defaultValue)
}

func useContext<T>(_ ctx: Context<T>) -> T {
    return ctx._current
}

func use<T>(_ ctx: Context<T>) -> T {
    return ctx._current
}

class Component<P, S> {
    var props: P!
    var state: S!
    var _host: Node? = nil
    var _renderFn: (() -> Node)? = nil

    func _mount(_ props: P, _ renderFn: @escaping () -> Node) -> Node {
        self.props = props
        self._renderFn = renderFn
        let host = Node()
        self._host = host
        host.syncFrom(renderFn())
        return host
    }

    func setState(_ next: S) {
        self.state = next
        if let h = _host {
            if let r = _renderFn {
                h.syncFrom(r())
            }
        }
    }

    func componentDidMount() {}
    func componentDidUpdate(prevProps: P, prevState: S) {}
    func componentWillUnmount() {}
    func render() -> Node { return Node() }
}

func Fragment(children: Node = nil) -> Node {
    return children
}

func Suspense(fallback: Node = nil, children: Node = nil) -> Node {
    return children
}

func StrictMode(children: Node = nil) -> Node {
    return children
}

func Profiler(id: string, onRender: ((string, string, float64) -> void)? = nil, children: Node = nil) -> Node {
    return children
}

enum component {
    struct Attribute {
        let apply: (Node) -> void

        static func Static(_ name: string, _ value: string) -> Attribute {
            return Attribute(apply: { n in
                if name == "class" || name == "className" {
                    if !value.isEmpty { n.Classes.append(value) }
                } else {
                    n.setAttr(name, value)
                }
            })
        }

        static func Value<V>(_ name: string, _ value: V) -> Attribute {
            return Attribute(apply: { n in
                if let b = value as? bool {
                    if b {
                        n.setAttr(name, "\u{0}")
                    } else {
                        n.removeAttr(name)
                    }
                } else if let st = value as? Style {
                    for (k, v) in st.pairs() {
                        n.setStyle(k, v)
                    }
                } else if let dict = value as? [string: string] {
                    if name == "style" {
                        for k in dict.keys.sorted() {
                            n.setStyle(k, dict[k]!)
                        }
                    }
                } else if name == "class" || name == "className" {
                    let s = "\(value)"
                    if !s.isEmpty { n.Classes.append(s) }
                } else {
                    n.setAttr(name, "\(value)")
                }
            })
        }

        static func On<E: dom.Event>(_ name: string, _ type: E.Type, _ handler: @escaping (E) -> void) -> Attribute {
            return Attribute(apply: { n in
                n.Handlers.append((name, { _ in
                    let e = _vsxMakeEvent(name)
                    if let typed = e as? E { handler(typed) }
                }))
            })
        }

        static func Live<V>(_ name: string, _ f: () -> V) -> Attribute {
            return Value(name, f())
        }

        static func Key<V>(_ key: V) -> Attribute {
            return Attribute(apply: { n in
                n.Key = "\(key)"
            })
        }

        static func Ref<V>(_ ref: V) -> Attribute {
            return Attribute(apply: { _ in })
        }

        static func Package(_ sheet: string) -> Attribute {
            return Attribute(apply: { _ in })
        }

        static func Spread(_ attrs: [string: string]) -> Attribute {
            return Attribute(apply: { n in
                for k in attrs.keys.sorted() {
                    let v = attrs[k]!
                    if k == "class" || k == "className" {
                        if !v.isEmpty { n.Classes.append(v) }
                    } else {
                        n.setAttr(k, v)
                    }
                }
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

    static func Live<V: Renderable>(_ f: () -> V) -> Node {
        return Fragment([f()])
    }

    static func Component(_ f: @escaping () -> Node) -> Node {
        let host = Node()
        let fiber = _Fiber(renderFn: f, host: host)
        fiber.run()
        return host
    }

    static func Component<T>(_ f: () -> T) -> T {
        return f()
    }

    static func MountClass<C, P>(_ inst: C, _ props: P, _ mount: (C, P) -> Node) -> Node {
        return mount(inst, props)
    }

    static func Key<K, V>(_ key: K, _ node: V) -> V {
        return node
    }

    static func Ref<R, V>(_ ref: R, _ node: V) -> V {
        return node
    }

    static func Styled(_ sheet: string, _ node: Node) -> Node {
        return node
    }

    static func Fragment(_ children: [any Renderable]) -> Node {
        let n = Node()
        for c in children { n.Children += c.Nodes() }
        return n
    }
}

func _vsx(_ tag: string, _ attributes: [component.Attribute], _ children: [any Renderable]) -> Node {
    return component.Element(tag, attributes, children)
}

func _vsxs(_ tag: string, _ attributes: [component.Attribute], _ children: [any Renderable]) -> Node {
    return component.Element(tag, attributes, children)
}

func _Fragment(_ children: [any Renderable]) -> Node {
    return component.Fragment(children)
}
