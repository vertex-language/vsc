// §6.9 Macro Roles, Multi-Role Declarations, and Accessor Macros

@attached(accessor)
macro LoggedAccess() = #externalMacro(module: "LoggingMacros", type: "LoggedAccessMacro")

@attached(memberAttribute)
macro AutoCodable() = #externalMacro(module: "CodableMacros", type: "AutoCodableMacro")

@attached(member, names: named(init), arbitrary)
@attached(extension, conformances: Sendable, Identifiable)
macro CompositeModel() = #externalMacro(module: "ModelMacros", type: "CompositeModelMacro")

@CompositeModel
struct Product {
    var id: String

    @LoggedAccess
    var price: Double = 0.0
}

@AutoCodable
struct Order {
    var orderID: Int
    var items: [String]
}
