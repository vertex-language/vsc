// if and guard take lists of conditions: bindings, cases and booleans together.
enum Reply { case ok(Int), error(String) }
func handle(_ raw: String?, _ reply: Reply) -> String {
    if let raw, let n = Int(raw), n > 0, case .ok(let code) = reply, code == 200 {
        return "good \(n)"
    }
    guard let raw else { return "no input" }
    if case .error(let e) = reply { return "error \(e) for \(raw)" }
    return "rejected \(raw)"
}
print(handle("5", .ok(200)))
print(handle("5", .ok(404)))
print(handle(nil, .ok(200)))
print(handle("x", .error("down")))
