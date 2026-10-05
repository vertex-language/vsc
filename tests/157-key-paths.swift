// Key paths name a property, and read and write through it.
struct Person {
    var name: String
    var address: Address
}
struct Address { var city: String }
var p = Person(name: "ann", address: Address(city: "Porto"))
let cityPath = \Person.address.city
print(p[keyPath: \.name], p[keyPath: cityPath])
p[keyPath: cityPath] = "Braga"
print(p.address.city)
let people = [p, Person(name: "bo", address: Address(city: "Faro"))]
print(people.map(\.name), people.map(\.address.city))
