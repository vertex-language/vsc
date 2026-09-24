// §6.9 Macro Declarations and Expansions

@freestanding(expression)
macro stringify<T>(_ value: T) -> (T, String) = #externalMacro(module: "MyMacros", type: "StringifyMacro")

@freestanding(declaration, names: arbitrary)
macro createStruct(_ name: String) = #externalMacro(module: "MyMacros", type: "StructGeneratorMacro")

@attached(member, names: named(init), arbitrary)
macro AutoInit() = #externalMacro(module: "MyMacros", type: "AutoInitMacro")

@attached(peer, names: overloaded)
macro AddAsync() = #externalMacro(module: "MyMacros", type: "AddAsyncMacro")

@attached(extension, conformances: CustomStringConvertible)
macro AutoDescription() = #externalMacro(module: "MyMacros", type: "AutoDescriptionMacro")

@AutoInit
struct Customer {
    let id: Int
    let name: String
}

func testMacroExpansion() {
    let result = #stringify(2 + 2)
    print(result.0, result.1)
}
