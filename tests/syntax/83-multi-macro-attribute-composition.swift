// §6.9 Multiple Macro Attributes and Role Composition

@freestanding(expression)
macro URLValidator(_ string: String) -> Bool = #externalMacro(module: "ValidationMacros", type: "URLValidatorMacro")

@attached(member, names: named(init), named(shared), arbitrary)
@attached(extension, conformances: CustomStringConvertible)
macro ServiceSingleton() = #externalMacro(module: "ServiceMacros", type: "ServiceSingletonMacro")

@attached(accessor)
macro ObservableField() = #externalMacro(module: "ObservationMacros", type: "ObservableFieldMacro")

@ServiceSingleton
class ConfigurationService {
    @ObservableField
    var endpoint: String = "https://api.vertex.org"

    @ObservableField
    var retryLimit: Int = 5
}

func testMacroComposition() {
    let isValid = #URLValidator("https://vertex.org/docs")
    _ = isValid
}
