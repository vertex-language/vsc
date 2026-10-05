// An async method overridden in a subclass is called through the table,
// and awaited like any other async call.
class Animal {
    func sound() async -> String { return "..." }
    func name() -> String { return "animal" }
}
class Dog: Animal {
    override func sound() async -> String { return "woof" }
    override func name() -> String { return "dog" }
}
func run() async {
    let a: Animal = Dog()
    print(await a.sound(), a.name())
    let b = Animal()
    print(await b.sound(), b.name())
}
await run()
