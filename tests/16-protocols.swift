// §6.6 Protocols

protocol Vehicle {
    // ProtocolPropertyDeclaration with GetterSetterKeywordBlock
    var speed: Double { get set }
    var maxSpeed: Double { get }

    // ProtocolMethodDeclaration
    func accelerate(by amount: Double)

    // ProtocolInitializerDeclaration
    init(maxSpeed: Double)

    // ProtocolSubscriptDeclaration
    subscript(index: Int) -> Double { get }

    // ProtocolAssociatedTypeDeclaration
    associatedtype FuelType
}

// AssociatedType with TypeInheritanceClause and default (TypealiasAssignment)
protocol Container {
    associatedtype Item: Equatable = Int
    var items: [Item] { get }
}

// Protocol inheriting from other protocols (TypeInheritanceClause)
protocol NamedVehicle: Vehicle {
    var name: String { get }
}

// Conforming type
struct Car: NamedVehicle {
    var speed: Double = 0
    let maxSpeed: Double
    let name: String
    typealias FuelType = String

    init(maxSpeed: Double) {
        self.maxSpeed = maxSpeed
        self.name = "Car"
    }

    func accelerate(by amount: Double) { }
    subscript(index: Int) -> Double { speed }
}