// A generic class's computed property with a getter and a setter answers
// the specialized T, read and written, in generic code and out of it.
final class Cell<T> {
    private var stored: T
    init(_ v: T) { stored = v }
    var Value: T {
        get { stored }
        set { stored = newValue }
    }
}
let i = Cell(3)
i.Value += 4
print(i.Value)
let s = Cell("a")
s.Value = s.Value + "b"
print(s.Value, s.Value.count)
func double<T: Numeric>(_ c: Cell<T>) { c.Value = c.Value * 2 }
double(i)
print(i.Value)
let d = Cell(1.5)
double(d)
print(d.Value)
