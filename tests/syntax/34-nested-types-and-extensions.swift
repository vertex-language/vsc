// §6.5 / §6.8 Nested Types and Constrained Extensions

struct Graph<NodeData: Hashable> {
    struct Node: Hashable {
        let id: Int
        var data: NodeData
    }

    struct Edge: Hashable {
        let source: Node
        let destination: Node
        let weight: Double
    }

    var nodes: Set<Node> = []
    var edges: Set<Edge> = []
}

extension Graph.Node: CustomStringConvertible {
    var description: String {
        return "Node(\(id)): \(data)"
    }
}

extension Graph.Edge: Comparable {
    static func <(lhs: Graph.Edge, rhs: Graph.Edge) -> Bool {
        return lhs.weight < rhs.weight
    }
}

extension Array where Element: Equatable {
    func removingDuplicates() -> [Element] {
        var result = [Element]()
        for item in self {
            if !result.contains(item) {
                result.append(item)
            }
        }
        return result
    }
}

func localTypeExample() -> Int {
    struct LocalAccumulator {
        var sum: Int = 0
        mutating func add(_ x: Int) { sum += x }
    }

    var acc = LocalAccumulator()
    acc.add(10)
    acc.add(20)
    return acc.sum
}
