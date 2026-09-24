// Methods, mutating methods and static members on an enum.
enum Light {
    case red, yellow, green
    static let start = Light.red
    mutating func next() {
        switch self {
        case .red: self = .green
        case .green: self = .yellow
        case .yellow: self = .red
        }
    }
    func canGo() -> Bool { self == .green }
}
var l = Light.start
for _ in 0..<4 {
    print(l, l.canGo())
    l.next()
}
