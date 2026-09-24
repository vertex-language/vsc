// §9.2 Deep Generic Parameter Constraints and Cross-Parameter Where Clauses

protocol GraphStorage {
    associatedtype Vertex: Hashable
    associatedtype EdgeWeight: Numeric & Comparable
}

struct AdjacencyMatrix<Storage: GraphStorage>: GraphStorage {
    typealias Vertex = Storage.Vertex
    typealias EdgeWeight = Storage.EdgeWeight

    private var matrix: [Vertex: [Vertex: EdgeWeight]] = [:]
}

func shortestPath<
    G1: GraphStorage,
    G2: GraphStorage,
    V: Hashable,
    W: Numeric & Comparable
>(
    graphA: G1,
    graphB: G2,
    start: V,
    end: V
) -> W? where G1.Vertex == V, G2.Vertex == V, G1.EdgeWeight == W, G2.EdgeWeight == W {
    _ = (graphA, graphB, start, end)
    return nil
}

struct MultiCollectionZip<
    S1: Sequence,
    S2: Collection,
    S3: BidirectionalCollection
> where S1.Element == S2.Element, S2.Element == S3.Element, S2.Index == Int {
    var first: S1
    var second: S2
    var third: S3

    func matchFirst() -> S1.Element? {
        return second.first
    }
}
