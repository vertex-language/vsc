// A closure argument to a static method is read twice: once with no type,
// to pick the method, and again as the parameter says. The second reading
// is the one lowered: `captured.append(s)` is append(_:), not the generic
// append(contentsOf:) the first, with `s` still unknown, took it for.
struct Handler {
    let emit: ((String) -> Void)?
    static func make(level: Int? = nil, emit: ((String) -> Void)? = nil) -> Handler {
        return Handler(emit: emit)
    }
    static func plain(_ emit: @escaping (String) -> Void) -> Handler {
        return Handler(emit: emit)
    }
}
var captured: [String] = []
let a = Handler.make(level: 1, emit: { s in
    captured.append(s)
})
a.emit?("one")
let b = Handler.plain { s in captured.append(s + "!") }
b.emit?("two")
print(captured)
