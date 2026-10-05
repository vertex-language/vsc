// Raw values of Character, Double and String, and enums with static and computed members.
enum Glyph: Character { case star = "*", dash = "-", dot = "." }
enum Rate: Double { case low = 0.5, high = 2.25 }
enum Mode: String, CaseIterable {
    case fast, safe = "SAFE"
    static var fallback: Mode { .safe }
    var label: String { rawValue.uppercased() }
}
print(Glyph.star.rawValue, Glyph(rawValue: ".") as Any, Rate.high.rawValue * 2, Rate(rawValue: 0.5) as Any)
print(Mode.allCases.map(\.rawValue), Mode.fallback, Mode.fast.label, Mode(rawValue: "fast") as Any)
