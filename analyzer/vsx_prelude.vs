// Built-in .vsx v2 prelude: UI types, DOM events/elements, hooks, and
// element/component lowering targets available in every .vsx compilation unit.

/// The events a handler is given, as web/dom has them.
class Event {
    var Type: string = ""
    var DefaultPrevented: bool = false
    var PropagationStopped: bool = false
    var Bubbles: bool = true
    init() {}
    func PreventDefault() { DefaultPrevented = true }
    func StopPropagation() { PropagationStopped = true }
}

class MouseEvent: Event {
    var X: float32 = 0
    var Y: float32 = 0
    var Clicks: int32 = 1
    var Button: int32 = 0
    var Pressed: bool = false
    var Shift: bool = false
    var Control: bool = false
    var Alt: bool = false
    var Meta: bool = false
}

final class PointerEvent: MouseEvent {
    var PointerType: string = "mouse"
    var PointerId: int32 = 1
    var IsPrimary: bool = true
    var Pressure: float32 = 0
    var TiltX: float32 = 0
    var TiltY: float32 = 0
}

final class WheelEvent: Event {
    var X: float32 = 0
    var Y: float32 = 0
    var DeltaX: float32 = 0
    var DeltaY: float32 = 0
    var Shift: bool = false
    var Control: bool = false
    var Alt: bool = false
    var Meta: bool = false
}

final class KeyboardEvent: Event {
    var Key: string = ""
    var Code: string = ""
    var Shift: bool = false
    var Control: bool = false
    var Alt: bool = false
    var Meta: bool = false
    var Repeat: bool = false
}

final class InputEvent: Event {
    var Value: string = ""
    var Data: string = ""
}

final class SubmitEvent: Event {}
final class FocusEvent: Event {}
final class CompositionEvent: Event {}
final class DataTransfer {
    var DropEffect: string = "none"
    var EffectAllowed: string = "uninitialized"
    var Files: [string] = []
    var Types: [string] = []
    init() {}
    func SetData(_ type: string, _ data: string) {}
    func GetData(_ type: string) -> string { return "" }
    func ClearData(_ type: string? = nil) {}
}

final class DragEvent: MouseEvent {
    var Data: DataTransfer = DataTransfer()
}

final class ToggleEvent: Event {
    var OldState: string = ""
    var NewState: string = ""
}

final class TransitionEvent: Event {
    var PropertyName: string = ""
    var ElapsedTime: float64 = 0
}

final class AnimationEvent: Event {
    var AnimationName: string = ""
    var ElapsedTime: float64 = 0
}
final class LayoutEvent: Event {}

func _vsxMakeEvent(_ type: string) -> Event {
    var e: Event
    switch type {
    case "click", "dblclick", "contextmenu", "mousedown", "mouseup", "mousemove", "mouseenter", "mouseleave", "mouseover", "mouseout":
        e = MouseEvent()
    case "pointerdown", "pointerup", "pointermove", "pointerenter", "pointerleave", "pointercancel":
        e = PointerEvent()
    case "wheel":
        e = WheelEvent()
    case "keydown", "keyup", "keypress":
        e = KeyboardEvent()
    case "input", "beforeinput":
        e = InputEvent()
    case "submit":
        e = SubmitEvent()
    case "focus", "blur":
        e = FocusEvent()
    case "compositionupdate", "compositionstart", "compositionend":
        e = CompositionEvent()
    case "drag", "dragstart", "dragover", "drop", "dragenter", "dragleave", "dragend":
        e = DragEvent()
    case "toggle", "beforetoggle":
        e = ToggleEvent()
    case "transitionrun", "transitionstart", "transitionend", "transitioncancel":
        e = TransitionEvent()
    case "animationstart", "animationiteration", "animationend", "animationcancel":
        e = AnimationEvent()
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
        if _vsxVoid(Tag) { return s }
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
typealias HTMLElement = Node
typealias HTMLInputElement = Node
typealias HTMLButtonElement = Node
typealias HTMLDivElement = Node
typealias HTMLSpanElement = Node
typealias HTMLFormElement = Node

/// An element that has no end tag and no children: `<br>`, `<input>`.
func _vsxVoid(_ tag: string) -> bool {
    switch tag {
    case "area", "base", "br", "col", "embed", "hr", "img", "input", "link", "meta", "source", "track", "wbr":
        return true
    default:
        return false
    }
}

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
    init(_ value: string) { self.raw = value }
    init(_ value: int) { self.raw = "\(value)px" }
    init(_ value: float64) { self.raw = "\(value)" }
    init(_ value: CSSValue) { self.raw = value.raw }
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

final class Ref<T> {
    var current: T?
    init(_ current: T? = nil) {
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

/// What useState gives to change its state: `setCount(3)`, or
/// `setCount { $0 + 1 }` from what it is when the setter runs.
struct SetState<T> {
    let cell: _HookCell<T>
    let fiber: _Fiber?

    func callAsFunction(_ next: T) {
        cell.value = next
        if let f = fiber { f.run() }
    }

    func callAsFunction(_ update: (T) -> T) {
        callAsFunction(update(cell.value))
    }
}

func useState<T>(_ initial: T) -> (T, SetState<T>) {
    if let fiber = _currentFiber {
        let idx = fiber.index
        fiber.index += 1
        if idx >= fiber.hooks.count {
            fiber.hooks.append(_HookCell<T>(initial))
        }
        if let cell = fiber.hooks[idx] as? _HookCell<T> {
            return (cell.value, SetState<T>(cell: cell, fiber: fiber))
        }
    }
    let cell = _HookCell<T>(initial)
    return (cell.value, SetState<T>(cell: cell, fiber: nil))
}

func useState<T>(_ initial: () -> T) -> (T, SetState<T>) {
    if let fiber = _currentFiber, fiber.index < fiber.hooks.count, let cell = fiber.hooks[fiber.index] as? _HookCell<T> {
        fiber.index += 1
        return (cell.value, SetState<T>(cell: cell, fiber: fiber))
    }
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

func useRef<T>(_ initial: T? = nil) -> Ref<T> {
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

final class _EffectSlot: _HookSlot {
    var deps: [any Equatable]?
    var cleanup: (() -> void)? = nil
    init(_ deps: [any Equatable]?) {
        self.deps = deps
        super.init()
    }
}

final class _MemoSlot<T>: _HookSlot {
    var value: T
    var deps: [any Equatable]?
    init(_ value: T, _ deps: [any Equatable]?) {
        self.value = value
        self.deps = deps
        super.init()
    }
}

func _sameDep(_ a: any Equatable, _ b: any Equatable) -> bool {
    if let x = a as? int, let y = b as? int { return x == y }
    if let x = a as? string, let y = b as? string { return x == y }
    if let x = a as? bool, let y = b as? bool { return x == y }
    if let x = a as? float64, let y = b as? float64 { return x == y }
    return false
}

func _depsEqual(_ a: [any Equatable]?, _ b: [any Equatable]?) -> bool {
    guard let x = a, let y = b else { return false }
    if x.count != y.count { return false }
    for i in 0..<x.count {
        if !_sameDep(x[i], y[i]) { return false }
    }
    return true
}

func _scheduleEffect(_ deps: [any Equatable]?, _ run: @escaping (_EffectSlot) -> void) {
    guard let fiber = _currentFiber else {
        run(_EffectSlot(deps))
        return
    }
    let idx = fiber.index
    fiber.index += 1
    if idx >= fiber.hooks.count {
        let slot = _EffectSlot(deps)
        fiber.hooks.append(slot)
        fiber.effects.append({ run(slot) })
        return
    }
    if let slot = fiber.hooks[idx] as? _EffectSlot {
        if _depsEqual(slot.deps, deps) { return }
        slot.deps = deps
        fiber.effects.append({
            if let c = slot.cleanup {
                slot.cleanup = nil
                c()
            }
            run(slot)
        })
    }
}

func useEffect(_ effect: @escaping () -> void, _ deps: [any Equatable]? = nil) {
    _scheduleEffect(deps, { _ in effect() })
}

func useEffect(_ effect: @escaping () -> (() -> void), _ deps: [any Equatable]? = nil) {
    _scheduleEffect(deps, { slot in slot.cleanup = effect() })
}

func useLayoutEffect(_ effect: @escaping () -> void, _ deps: [any Equatable]? = nil) {
    useEffect(effect, deps)
}

func useInsertionEffect(_ effect: @escaping () -> void, _ deps: [any Equatable]? = nil) {
    useEffect(effect, deps)
}

func useMemo<T>(_ compute: () -> T, _ deps: [any Equatable]? = nil) -> T {
    guard let fiber = _currentFiber else { return compute() }
    let idx = fiber.index
    fiber.index += 1
    if idx < fiber.hooks.count, let slot = fiber.hooks[idx] as? _MemoSlot<T> {
        if !_depsEqual(slot.deps, deps) {
            slot.value = compute()
            slot.deps = deps
        }
        return slot.value
    }
    let slot = _MemoSlot<T>(compute(), deps)
    fiber.hooks.append(slot)
    return slot.value
}

func useCallback<F>(_ callback: F, _ deps: [any Equatable]? = nil) -> F {
    return useMemo({ callback }, deps)
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

final class Context<T> {
    let defaultValue: T
    var _current: T
    init(_ defaultValue: T) {
        self.defaultValue = defaultValue
        self._current = defaultValue
    }
    func Provider(value: T, children: () -> Node) -> Node {
        return component.Call("Context.Provider", {
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

        static func OnCapture<E: dom.Event>(_ name: string, _ type: E.Type, _ handler: @escaping (E) -> void) -> Attribute {
            return On(name, type, handler)
        }

        static func Live<V>(_ name: string, _ f: () -> V) -> Attribute {
            return Value(name, f())
        }

        static func Key<V>(_ key: V) -> Attribute {
            return Attribute(apply: { n in
                n.Key = "\(key)"
            })
        }

        static func Ref<T>(_ ref: Ref<T>) -> Attribute {
            return Attribute(apply: { _ in })
        }

        static func Ref<T>(_ setter: SetState<T?>) -> Attribute {
            return Attribute(apply: { _ in })
        }

        static func Ref(_ callback: @escaping (Node?) -> void) -> Attribute {
            return Attribute(apply: { n in callback(n) })
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

    static func Call(_ type: string, _ f: @escaping () -> Node) -> Node {
        let host = Node()
        let fiber = _Fiber(renderFn: f, host: host)
        fiber.run()
        return host
    }

    static func Call<T>(_ type: string, _ f: () -> T) -> T {
        return f()
    }

    static func MountClass<C, P>(_ type: string, _ inst: C, _ props: P, _ mount: (C, P) -> Node) -> Node {
        return mount(inst, props)
    }

    static func MountClass<K, C, P>(key: K, _ type: string, _ inst: C, _ props: P, _ mount: (C, P) -> Node) -> Node {
        let n = mount(inst, props)
        n.Key = "\(key)"
        return n
    }

    static func Key<K>(_ key: K, _ type: string, _ f: @escaping () -> Node) -> Node {
        let n = Call(type, f)
        n.Key = "\(key)"
        return n
    }

    static func Key<K, V>(_ key: K, _ type: string, _ f: () -> V) -> V {
        return f()
    }

    static func ForwardRef<R, V>(_ ref: R, _ node: V) -> V {
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
