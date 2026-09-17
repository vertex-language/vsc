// §2.3.2 Custom String Interpolation Protocol Extensions

extension String.StringInterpolation {
    mutating func appendInterpolation(format value: Double, precision: Int = 2) {
        let formatted = String(format: "%.\(precision)f", value)
        appendLiteral(formatted)
    }

    mutating func appendInterpolation(pad text: String, toLength length: Int) {
        if text.count < length {
            appendLiteral(text + String(repeating: " ", count: length - text.count))
        } else {
            appendLiteral(text)
        }
    }

    mutating func appendInterpolation<T: CustomStringConvertible>(optional: T?, default fallback: String = "nil") {
        if let val = optional {
            appendLiteral(val.description)
        } else {
            appendLiteral(fallback)
        }
    }
}

func testCustomInterpolation() {
    let pi = 3.14159265
    let message = "Value: \(format: pi, precision: 3)"
    let padded = "Item: [\(pad: "code", toLength: 8)]"
    let opt: Int? = nil
    let fallbackText = "Result: \(optional: opt, default: "N/A")"
    _ = (message, padded, fallbackText)
}
