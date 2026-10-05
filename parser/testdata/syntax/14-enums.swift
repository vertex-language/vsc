// §6.5 Enums

// Simple EnumCaseDeclaration
enum Compass {
    case north
    case south, east, west
}

// RawValueAssignment with RawValueLiteral (numeric, string, boolean)
enum StatusCode: Int {
    case ok = 200
    case notFound = 404
    case serverError = 500
}

enum Setting: String {
    case enabled = "on"
    case disabled = "off"
}

// EnumCasePattern with TuplePattern (associated values)
enum Barcode {
    case upc(Int, Int, Int, Int)
    case qrCode(String)
}

// indirect case
indirect enum Expression {
    case number(Int)
    case addition(Expression, Expression)
}

// Enum conforming to protocols via TypeInheritanceClause
protocol Describable { var description: String { get } }
enum Coin: Int, Describable {
    case penny = 1, nickel = 5, dime = 10, quarter = 25
    var description: String { "Worth \(rawValue) cents" }
}

// Using patterns to match enum cases
func describe(_ code: StatusCode) -> String {
    switch code {
    case .ok: return "OK"
    case .notFound: return "Not Found"
    case .serverError: return "Server Error"
    }
}