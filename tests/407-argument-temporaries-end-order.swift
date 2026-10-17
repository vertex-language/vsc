// Temporaries made for one statement's call arguments end in reverse:
// the later argument's first.
final class Circle {
    let r: Int
    init(_ r: Int) { self.r = r }
    deinit { print("circle", r, "freed") }
    func area() -> Int { 3 * r * r }
}
func f(_ c: Circle) -> Circle { c }
print(f(Circle(5)).area(), f(Circle(6)).area())
print("done")
