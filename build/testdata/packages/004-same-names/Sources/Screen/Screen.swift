public enum Cursor: Equatable {
    case arrow
    case hidden
}

public struct Window {
    public var cursor: Cursor = .arrow

    public init() {}

    public mutating func setCursor(_ c: Cursor) {
        cursor = c
    }
}

public func describe(_ c: Cursor) -> String {
    return c == .hidden ? "hidden" : "arrow"
}
