// §2.2 / §6.5 Raw Identifiers in Complex Declarations

enum ReservedTokenKind {
    case `associatedtype`
    case `subscript`
    case `where`
    case `default`
    case `rethrows`
    case `throws`
}

protocol KeywordProtocol {
    func `catch`(error: Error)
    func `throw`() throws
}

struct KeywordContainer: KeywordProtocol {
    var `self`: Int
    var `Type`: String

    subscript(`in` domain: String, `for` id: Int) -> String {
        get { "\(domain):\(id):\(self.`self`)" }
        set { self.`Type` = newValue }
    }

    func `catch`(error: Error) {
        print(error)
    }

    func `throw`() throws {
        // ok
    }
}

func testKeywordContainer() {
    var c = KeywordContainer(self: 1, Type: "test")
    c[in: "local", for: 42] = "updated"
    let token = ReservedTokenKind.`associatedtype`
    _ = (c, token)
}
