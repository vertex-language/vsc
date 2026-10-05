// §4.6 Complex Closures and Capture Lists

class Handler {
    var id: Int = 1
    var delegate: Handler?

    func setup() {
        let closure = { [weak self, unowned(unsafe) del = self?.delegate, copyID = self?.id] (delta: Int) -> Int in
            guard let self = self else { return 0 }
            _ = del
            return (copyID ?? self.id) + delta
        }
        _ = closure(5)
    }
}

func higherOrder(
    builder: (Int) -> (String) -> Bool,
    transform: @escaping @Sendable (Int) async throws -> String
) {
    let sub = builder(10)
    _ = sub("test")
}

func multiTrailing(
    action: () -> Void,
    onSuccess: (Int) -> Void,
    onFailure: (Error) -> Void
) {
    action()
}

func testMultiTrailing() {
    multiTrailing {
        print("action")
    } onSuccess: { code in
        print("success: \(code)")
    } onFailure: { error in
        print("error: \(error)")
    }
}

func autoClosureExample(predicate: @autoclosure () -> Bool) -> Bool {
    return predicate()
}
