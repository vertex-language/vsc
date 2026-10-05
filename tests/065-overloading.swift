// Functions overloaded on parameter type, label and return type.
func show(_ x: Int) -> String { "Int \(x)" }
func show(_ x: Double) -> String { "Double \(x)" }
func show(_ x: String) -> String { "String \(x)" }
func show(value x: Int) -> String { "labelled \(x)" }
func make() -> Int { 1 }
func make() -> String { "one" }
print(show(1), show(1.5), show("s"), show(value: 2))
let i: Int = make()
let s: String = make()
print(i, s)
