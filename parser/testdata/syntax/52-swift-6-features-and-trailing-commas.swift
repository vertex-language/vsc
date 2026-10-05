// Swift 6.1 Concrete Compiler Features Verified with swiftc

import Foundation

// 1. Extended Trailing Commas
func computeValues(
    alpha: Int,
    beta: String,
) -> (Int, String,) {
    return (alpha, beta,)
}

struct MultiContainer<First, Second,> {
    var pair: (First, Second,)

    func callWithTrailing() {
        let _ = computeValues(
            alpha: 10,
            beta: "test",
        )
    }
}

func testCaptureListTrailing() {
    let x = 1
    let y = 2
    let closure = { [x, y,] in
        print(x + y)
    }
    closure()
}

// 2. Sending Parameter and Result
func sendBuffer(_ buffer: sending [UInt8]) -> sending [UInt8] {
    return buffer
}

// 3. Nonisolated on Type and Extension
nonisolated struct StandaloneRecord {
    var id: Int
}

nonisolated extension StandaloneRecord {
    func display() -> String {
        return "Record: \(id)"
    }
}

// 4. Isolated(any) Function Type
func acceptIsolatedFn(fn: @isolated(any) () -> Void) {
    fn()
}

// 5. Objective-C Category Implementation
@implementation extension NSObject {
    func customSwiftMethod() -> Int {
        return 42
    }
}

// 6. Local nonisolated(unsafe) variable
func testLocalNonisolated() {
    nonisolated(unsafe) var localCounter = 0
    localCounter += 1
    _ = localCounter
}

// 7. Consume inout parameter
func testConsumeInout(val: inout Int) {
    let taken = consume val
    val = 100
    _ = taken
}

