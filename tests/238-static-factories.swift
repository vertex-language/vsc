// Static factories and presets reached with a leading dot.
struct Color: Equatable, CustomStringConvertible {
    let r, g, b: UInt8
    static let black = Color(r: 0, g: 0, b: 0)
    static let white = Color(r: 255, g: 255, b: 255)
    static func gray(_ v: UInt8) -> Color { Color(r: v, g: v, b: v) }
    var description: String { "#" + [r, g, b].map { String($0, radix: 16).count == 1 ? "0" + String($0, radix: 16) : String($0, radix: 16) }.joined() }
}
func paint(_ c: Color) -> String { "painted \(c)" }
print(paint(.black), paint(.gray(128)), paint(.white), Color.gray(0) == .black)
