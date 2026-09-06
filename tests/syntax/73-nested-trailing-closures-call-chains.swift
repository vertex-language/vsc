// §4.6 Nested Trailing Closures and Deep Call Chains

func requestPipeline(
    initial: Int,
    preprocess: (Int) -> Int,
    transform: (Int) -> String,
    onSuccess: (String) -> Void,
    onError: (Error) -> Void
) {
    let prep = preprocess(initial)
    let res = transform(prep)
    onSuccess(res)
}

func nestedDispatcher(
    step: () -> Void,
    then: () -> Void
) {
    step()
    then()
}

func testNestedTrailingClosures() {
    nestedDispatcher {
        print("Outer step")
        requestPipeline(initial: 100) { raw in
            raw * 2
        } transform: { num in
            "Transformed: \(num)"
        } onSuccess: { str in
            nestedDispatcher {
                print("Inner success: \(str)")
            } then: {
                print("Inner completion")
            }
        } onError: { err in
            print("Failed: \(err)")
        }
    } then: {
        print("Outer then")
    }

    let chained = [1, 2, 3, 4, 5]
        .map { $0 * 2 }
        .filter { $0 > 4 }
        .reduce(0) { acc, nextVal in acc + nextVal }
    _ = chained
}
