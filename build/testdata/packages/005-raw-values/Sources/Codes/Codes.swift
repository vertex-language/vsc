public enum Kind: Int {
    case normal = 1
    case unknown
    case control = 3
    case byte = 6
}

public enum Tag: String {
    case bold = "b"
    case italic
}

public enum Level: UInt8 {
    case low = 7
    case high
}
