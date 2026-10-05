// §2.2 Identifiers

// IdentifierHead {IdentifierCharacter}
let camelCase = 1
let _underscoreStart = 2
let mixedCase123 = 3

// Backtick-escaped identifiers: `IdentifierHead {IdentifierCharacter}`
let `class` = "reserved word as identifier"
let `for` = "another reserved word"

// ImplicitParameterName: $ {DecimalDigit}
let addClosure: (Int, Int) -> Int = { $0 + $1 }

// PropertyWrapperProjection: $ IdentifierCharacters
@propertyWrapper
struct Wrapped<T> {
    var wrappedValue: T
    var projectedValue: String { "projected" }
}

struct HasWrapper {
    @Wrapped var value: Int = 10
    func show() {
        print(value)          // wrappedValue
        print($value)         // PropertyWrapperProjection
    }
}