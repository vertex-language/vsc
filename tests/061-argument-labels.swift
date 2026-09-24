// Argument labels: default, custom, and omitted with _.
func greet(person: String, from hometown: String) -> String {
    "Hello \(person) from \(hometown)"
}
func add(_ a: Int, _ b: Int) -> Int { a + b }
func move(from a: Int, to b: Int) -> Int { b - a }
print(greet(person: "Ana", from: "Lisbon"))
print(add(2, 3), move(from: 3, to: 10))
