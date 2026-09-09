// §6.8 Multiple Conditional Conformances on Standard Collections

protocol Serializable {
    func serialize() -> String
}

extension Int: Serializable {
    func serialize() -> String { String(self) }
}

extension String: Serializable {
    func serialize() -> String { self }
}

extension Optional: Serializable where Wrapped: Serializable {
    func serialize() -> String {
        switch self {
        case .some(let val): return "some(\(val.serialize()))"
        case .none: return "none"
        }
    }
}

extension Array: Serializable where Element: Serializable {
    func serialize() -> String {
        return "[" + self.map { $0.serialize() }.joined(separator: ", ") + "]"
    }
}

extension Dictionary: Serializable where Key: Serializable, Value: Serializable {
    func serialize() -> String {
        let entries = self.map { "\($0.key.serialize()): \($0.value.serialize())" }
        return "{\(entries.joined(separator: ", "))}"
    }
}

func printSerialized<T: Serializable>(_ item: T) {
    print(item.serialize())
}
