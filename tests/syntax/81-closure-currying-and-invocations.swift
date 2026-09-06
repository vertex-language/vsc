// §4.6 Curried Closures, Chained Invocations, and Immediately Invoked Closures

func curriedAdd(_ a: Int) -> (Int) -> (Int) -> Int {
    return { b in
        return { c in
            a + b + c
        }
    }
}

func builderWithImmediate(initial: Int, configure: (inout Int) -> () -> String) -> String {
    var state = initial
    let finalizer = configure(&state)
    return finalizer()
}

func testCurryingAndInvocations() {
    let result = curriedAdd(10)(20)(30)

    let immediateVal = { (fn: () -> Int) -> Int in
        fn() * 3
    } {
        14
    }

    let inlineConfig = builderWithImmediate(initial: 5) { current in
        current += 10
        return { "Final State: \(current)" }
    }

    let closureDict: [String: (Int) -> Int] = [
        "double": { $0 * 2 },
        "square": { $0 * $0 }
    ]
    let calculated = closureDict["double"]?(5) ?? 0

    _ = (result, immediateVal, inlineConfig, calculated)
}
