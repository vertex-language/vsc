// Assigning a stored property through a type parameter constrained to a class.
struct Header { var marked = false; var age = 0 }
class Cell { var header = Header() }
final class Str: Cell { let s: String; init(_ s: String) { self.s = s } }
func mark<T: Cell>(_ cell: T) {
    cell.header = Header(marked: true, age: cell.header.age + 1)
    cell.header.age += 1
}
let s = Str("x")
mark(s)
print(s.header.marked, s.header.age, s.s)
