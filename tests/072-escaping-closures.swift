// @escaping closures stored and called after the function returned.
var handlers: [() -> Void] = []
func register(_ name: String, _ h: @escaping () -> Void) {
    print("registered", name)
    handlers.append(h)
}
for i in 1...3 {
    register("h\(i)") { print("handler", i) }
}
for h in handlers { h() }
