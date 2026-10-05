// A protocol states requirements; conforming types meet them.
protocol Named {
    var name: String { get }
    func greet() -> String
}
struct Person: Named {
    let name: String
    func greet() -> String { "hi, I'm \(name)" }
}
class Robot: Named {
    var name: String { "robot" }
    func greet() -> String { "BEEP" }
}
func introduce<T: Named>(_ x: T) -> String { x.name + ": " + x.greet() }
print(introduce(Person(name: "ann")), introduce(Robot()))
