// §9.2 / §3.3 Recursive Generics, F-Bounded Polymorphism, and Nested Metatypes

protocol ComparableNode {
    associatedtype Payload: Comparable
    func compare(with other: Self) -> Bool
}

class BinaryTreeNode<T: Comparable>: ComparableNode {
    typealias Payload = T
    var value: T
    var left: BinaryTreeNode<T>?
    var right: BinaryTreeNode<T>?

    init(value: T) {
        self.value = value
    }

    func compare(with other: BinaryTreeNode<T>) -> Bool {
        return self.value < other.value
    }
}

protocol ComponentFactory {
    associatedtype Target
    static func create() -> Target
}

func inspectMetatypes<T: ComponentFactory>(factory: T.Type) -> (Any.Type, Any.Type) {
    let targetType: T.Target.Type = T.Target.self
    let factoryMeta: T.Type.Type = T.Type.self
    _ = factoryMeta
    return (targetType, T.self)
}

protocol Renderable {}
protocol Stylable {}

func testMetatypeCasting(instance: Any, meta: (any (Renderable & Stylable)).Type) -> Bool {
    if let _ = instance as? any Renderable & Stylable {
        return true
    }
    return false
}
