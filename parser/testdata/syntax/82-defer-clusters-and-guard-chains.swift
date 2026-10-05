// §5.1 / §5.2 Stacked Defers, Guard Chains, and Labeled Control Transfers

func executeComplexLifecycle(
    data: [String: Any]?,
    threshold: Int,
    rawTokens: [String]
) -> [String] {
    var auditLog: [String] = []

    defer { auditLog.append("L1: Final cleanup") }
    defer { auditLog.append("L2: Secondary cleanup") }

    guard
        let dict = data,
        !dict.isEmpty,
        let count = dict["count"] as? Int,
        count >= threshold,
        case 0...1000 = count
    else {
        auditLog.append("Guard failed")
        return auditLog
    }

    defer { auditLog.append("L3: Processed dictionary with count=\(count)") }

    outer: for (idx, token) in rawTokens.enumerated() {
        defer { auditLog.append("Loop defer for token \(idx)") }

        guard !token.isEmpty else {
            continue outer
        }

        var chars = Array(token)
        inner: while !chars.isEmpty {
            let ch = chars.removeFirst()
            if ch == "#" {
                break outer
            }
            if ch == " " {
                continue inner
            }
            auditLog.append("Char: \(ch)")
        }
    }

    return auditLog
}
