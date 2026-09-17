// §6.5 Advanced Enums

enum BinaryTree<Element: Comparable> {
    case empty
    indirect case node(left: BinaryTree<Element>, value: Element, right: BinaryTree<Element>)

    var count: Int {
        switch self {
        case .empty:
            return 0
        case let .node(left, _, right):
            return 1 + left.count + right.count
        }
    }

    func contains(_ target: Element) -> Bool {
        switch self {
        case .empty:
            return false
        case let .node(left, val, right):
            if target == val { return true }
            return target < val ? left.contains(target) : right.contains(target)
        }
    }
}

enum HTTPMethod: String, CaseIterable {
    case get = "GET"
    case post = "POST"
    case put = "PUT"
    case delete = "DELETE"
    case head, options, trace, patch
}

enum JSONValue: Equatable {
    case string(String)
    case number(Double)
    case boolean(Bool)
    case null
    indirect case array([JSONValue])
    indirect case object([String: JSONValue])

    subscript(key: String) -> JSONValue? {
        guard case let .object(dict) = self else { return nil }
        return dict[key]
    }

    subscript(index: Int) -> JSONValue? {
        guard case let .array(arr) = self, index >= 0, index < arr.count else { return nil }
        return arr[index]
    }
}
