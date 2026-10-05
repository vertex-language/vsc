// Protocol requirements for settable properties and mutating methods.
protocol Toggleable {
    var isOn: Bool { get set }
    mutating func toggle()
}
struct Switch: Toggleable {
    var isOn = false
    mutating func toggle() { isOn.toggle() }
}
func flipTwice<T: Toggleable>(_ x: inout T) -> [Bool] {
    x.toggle()
    let first = x.isOn
    x.isOn = !x.isOn
    return [first, x.isOn]
}
var s = Switch()
print(flipTwice(&s), s.isOn)
