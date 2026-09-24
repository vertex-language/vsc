public struct Cursor {
    public var width: Int

    public init(width: Int) {
        self.width = width
    }

    public func doubled() -> Cursor {
        return Cursor(width: width * 2)
    }
}
