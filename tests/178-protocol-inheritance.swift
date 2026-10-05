// A protocol that refines another, and dispatch to the most specific witness.
protocol Animal { func sound() -> String }
protocol Pet: Animal { var name: String { get } }
extension Animal { func speak() -> String { "... " + sound() } }
extension Pet { func speak() -> String { name + " says " + sound() } }
struct Wolf: Animal { func sound() -> String { "howl" } }
struct Dog: Pet { let name: String; func sound() -> String { "woof" } }
func viaAnimal<T: Animal>(_ a: T) -> String { a.speak() }
func viaPet<T: Pet>(_ p: T) -> String { p.speak() }
print(Wolf().speak(), Dog(name: "rex").speak())
print(viaAnimal(Dog(name: "rex")), viaPet(Dog(name: "rex")))
