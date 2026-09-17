class Animal {
    var name: String
    init(name: String) { self.name = name }
    func speak() -> String { return "" }
}
class Dog: Animal {
    override func speak() -> String { return "woof" }
}
func use(_ d: Dog) -> String {
    return d.name + d.speak()
}
