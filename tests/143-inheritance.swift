// A subclass inherits its superclass's properties and methods.
class Animal {
    var name: String
    init(name: String) { self.name = name }
    func describe() -> String { "\(name) the animal" }
}
class Dog: Animal {
    var tricks = 0
    func learn() { tricks += 1 }
}
let d = Dog(name: "rex")
d.learn(); d.learn()
print(d.describe(), d.tricks, d.name)
let a: Animal = d
print(a.describe())
