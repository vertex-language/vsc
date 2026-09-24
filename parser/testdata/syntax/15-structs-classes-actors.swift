// §6.5 Structs, Classes, and Actors

// StructDeclaration with GenericParameterClause
struct Stack<Element> {
    private var elements: [Element] = []
    mutating func push(_ element: Element) { elements.append(element) }
    mutating func pop() -> Element? { elements.popLast() }
}

// ClassDeclaration with TypeInheritanceClause (superclass + protocol)
protocol Identifiable2 { var id: Int { get } }

class Animal: Identifiable2 {
    let id: Int
    init(id: Int) { self.id = id }
    func speak() -> String { "..." }
}

class Dog: Animal {
    override func speak() -> String { "Woof" }
}

// final / dynamic / required modifiers
final class Sealed {}
class HasRequiredInit {
    required init() {}
}

// ActorDeclaration
actor Counter {
    private var value = 0
    func increment() { value += 1 }
    func current() -> Int { value }
}

// nonisolated / isolated modifiers on actor members
actor Cache {
    nonisolated let name: String
    private var storage: [String: Int] = [:]

    init(name: String) { self.name = name }

    func set(_ key: String, _ value: Int) { storage[key] = value }
}

// GenericWhereClause on a type declaration
struct Pair<A, B> where A: Hashable, B: Hashable {
    var first: A
    var second: B
}