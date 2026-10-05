// A lazy property's initializer runs once self exists, and a static one
// names static members: both may use the names an instance initializer
// may not.
struct Counter {
    var base = 2
    lazy var doubled: Int = twice()
    static let start = origin()

    func twice() -> Int { return base * 2 }
    static func origin() -> Int { return 1 }
}
