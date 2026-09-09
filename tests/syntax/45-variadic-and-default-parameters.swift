// §6.4 Variadic and Default Parameters

func sumAll(_ numbers: Double...) -> Double {
    return numbers.reduce(0.0, +)
}

func configure(
    timeout: Double = 30.0,
    retryCount: Int = 3,
    debug: Bool = false,
    headers: [String: String] = [:]
) -> String {
    return "timeout=\(timeout), retry=\(retryCount), debug=\(debug)"
}

func queryServer(
    for id: Int,
    in domain: String = "default",
    default fallback: String = "none"
) -> String {
    return "\(domain):\(id) (fallback: \(fallback))"
}

func keywordArguments(
    `let` first: Int,
    `var` second: Int,
    `func` third: () -> Void = {}
) {
    third()
    _ = (first, second)
}

func testCalls() {
    _ = sumAll(1.0, 2.0, 3.5)
    _ = configure(debug: true)
    _ = queryServer(for: 101, default: "cached")
    keywordArguments(let: 1, var: 2)
}
