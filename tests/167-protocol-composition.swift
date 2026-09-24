// P & Q asks for a value conforming to both.
protocol Named { var name: String { get } }
protocol Aged { var age: Int { get } }
struct Person: Named, Aged { let name: String; let age: Int }
struct Pet: Named { let name: String }
func card(_ x: some Named & Aged) -> String { "\(x.name), \(x.age)" }
let who: any Named & Aged = Person(name: "ann", age: 30)
print(card(Person(name: "bo", age: 4)), who.name, who.age)
let names: [any Named] = [Pet(name: "rex"), who]
print(names.map(\.name), names.map { $0 is any Aged })
