// §9.2 Complex Where Clauses and Chained Associated Types

protocol Container {
    associatedtype Item
    associatedtype SubContainer: Container where SubContainer.Item == Item
}

protocol Transformer {
    associatedtype Input
    associatedtype Output
}

struct Pipeline<C: Container, T: Transformer> where C.Item == T.Input, C.SubContainer.Item == T.Output {
    var container: C
    var transformer: T

    init(container: C, transformer: T) where T.Input: Hashable {
        self.container = container
        self.transformer = transformer
    }

    subscript<Idx: BinaryInteger>(index: Idx) -> T.Output where C.Item: Equatable {
        fatalError()
    }
}

extension Collection where Element: Collection, Element.Element: Equatable, Index == Int {
    func flattenedMatches(target: Element.Element) -> Bool {
        for sub in self {
            if sub.contains(target) {
                return true
            }
        }
        return false
    }
}
