// `a ?? b` whose type is an existential, with b a concrete value made into
// it: an error kept from a retry loop, a protocol value with a default.
enum Failure: Error { case none, code(Int) }

func lastOr(_ last: (any Error)?) -> any Error { last ?? Failure.code(2) }
print(lastOr(nil), lastOr(Failure.none))

func retry(_ attempts: [Int]) throws -> Int {
    var lastError: (any Error)? = nil
    for a in attempts {
        if a > 0 { return a }
        lastError = Failure.code(a)
    }
    throw lastError ?? Failure.none
}
do { _ = try retry([]) } catch { print("empty:", error) }
do { _ = try retry([0, -1]) } catch { print("failed:", error) }
print(try! retry([0, 5]))

protocol Shape { func area() -> Int }
struct Square: Shape { let s: Int; func area() -> Int { s * s } }
final class Circle: Shape {
    let r: Int
    init(_ r: Int) { self.r = r }
    func area() -> Int { 3 * r * r }
    deinit { print("circle", r, "freed") }
}
func shape(_ maybe: (any Shape)?) -> any Shape { maybe ?? Square(s: 3) }
print(shape(nil).area(), shape(Circle(2)).area())
let picked: any Shape = Optional<any Shape>.none ?? Circle(1)
print(picked.area())

// Returned, the temporary a `??`, an if expression or a parameter copy
// makes is moved out, so a class inside is freed when the caller is done.
func pass(_ m: any Shape) -> any Shape { return m }
func pick(_ b: Bool, _ m: any Shape) -> any Shape { if b { m } else { Square(s: 2) } }
func orSquare(_ m: (any Shape)?) -> any Shape { return m ?? Square(s: 3) }
print(pass(Circle(4)).area())
print(pick(true, Circle(5)).area())
print(pick(false, Circle(6)).area())
print(orSquare(Circle(7)).area())
print("done")
