// §4.8 Key-Path Expressions and Dynamic Member Lookup

struct Address {
    var street: String
    var zipCode: Int
}

struct Person {
    var name: String
    var address: Address
    var tags: [String]
}

let nameKeyPath = \Person.name
let streetKeyPath = \Person.address.street
let firstTagKeyPath = \Person.tags[0]
let implicitRootKeyPath: KeyPath<Person, String> = \.name
let arrayIndexKeyPath = \[Int].[0]

@dynamicMemberLookup
struct DynamicWrapper<T> {
    var value: T

    subscript<U>(dynamicMember member: KeyPath<T, U>) -> U {
        return value[keyPath: member]
    }
}

func testDynamicLookup(p: Person) {
    let wrapped = DynamicWrapper(value: p)
    _ = wrapped.name
    _ = wrapped.address.street
}
