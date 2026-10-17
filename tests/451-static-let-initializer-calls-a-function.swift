// A static let whose initializer calls a top-level function inside an
// initializer call: its type is known before the function is declared.
struct Theme {
    var name: String
    var fg: Int
    static let dark = Theme(name: "dark", fg: hex(0xCC))
    static let light = Theme(name: "light", fg: hex(0x33) + twice(1))
}
func hex(_ v: Int) -> Int { return v * 2 }
func twice(_ x: Int) -> Int { return x * 2 }
print(Theme.dark.name, Theme.dark.fg)
print(Theme.light.name, Theme.light.fg)
