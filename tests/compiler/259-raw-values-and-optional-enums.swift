// An enum made from its raw value, an optional of a plain enum, and a
// leading-dot case written as an operand.
//
// `KeyCode(rawValue: code) ?? .unknown` is how every mapping from a
// number on the wire to a case is written, and it is three things at once:
// a failable initializer every enum with a raw type has, an optional of an
// enum -- stored in the enum's own byte, with nil the first tag no case
// uses -- and a `.unknown` that takes its type from the other side of the
// `??`. `k == .escape` is the same last thing through `==`.
enum KeyCode: Int32 {
    case unknown = 0
    case a = 1
    case b = 2
    case escape = 40
    case enter = 41
}

enum Theme {
    case light, dark, highContrast
}

func code(_ raw: Int32) -> KeyCode {
    return KeyCode(rawValue: raw) ?? .unknown
}

func themeFor(_ n: Int) -> Theme? {
    switch n {
    case 0: return .light
    case 1: return .dark
    case 2: return .highContrast
    default: return nil
    }
}

func main() -> Int32 {
    var total: Int32 = 0

    // Hits and a miss.
    total += code(40).rawValue            // 40
    total += code(2).rawValue             // 2
    total += code(99).rawValue            // 0, through ??
    if KeyCode(rawValue: 7) == nil { total += 100 }
    if let k = KeyCode(rawValue: 41) { total += k.rawValue }   // 41

    // Leading dots on either side of == and !=.
    let k = code(40)
    if k == .escape { total += 1000 }
    if .escape == k { total += 1000 }
    if k != .enter { total += 1000 }

    // Optionals of a plain enum.
    var seen = 0
    for i in 0..<5 {
        if let t = themeFor(i) {
            seen += 1
            if t == .highContrast { total += 7 }
        }
    }
    total += Int32(seen) * 10000        // 30000
    let fallback = themeFor(9) ?? .dark
    if fallback == .dark { total += 5 }

    return total % 251
}
