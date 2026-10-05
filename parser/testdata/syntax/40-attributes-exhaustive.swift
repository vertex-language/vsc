// §7 Exhaustive Attributes

@frozen
enum Alignment {
    case left, center, right
}

@objcMembers
class ObjectiveCCompatible: NSObject {
    @objc var title: String = ""
    @objc func performAction() { }
}

@inlinable
func fastInlineAdd(_ a: Int, _ b: Int) -> Int {
    return a + b
}

@usableFromInline
internal var internalBuffer: [Int] = []

@inline(__always)
func forceInline() -> Int {
    return 42
}

@inline(never)
func neverInline() -> Int {
    return 0
}

@discardableResult
@warn_unqualified_access
func computeCritical() -> Int {
    return 100
}

let cFunctionPointer: @convention(c) (Int32, Int32) -> Int32 = { a, b in
    return a + b
}

func testUnknownDefault(a: Alignment) {
    switch a {
    case .left:
        print("left")
    case .center:
        print("center")
    case .right:
        print("right")
    @unknown default:
        print("unknown future case")
    }
}
