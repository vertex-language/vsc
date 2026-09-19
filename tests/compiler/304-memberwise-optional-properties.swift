// A memberwise initializer given a T for a property of type T? holds it
// wrapped, whether the T is a struct, a class or a value in a variable.

final class Box { var n = 1 }
struct I { var n = 0 }
struct H { var box: Box?; var inner: I?; var name: String? }

func main() -> Int32 {
    let i = I(n: 7)
    var h = H(box: Box(), inner: i, name: "x")
    let empty = H(box: nil, inner: nil, name: nil)
    h.box?.n = 4
    print(h.box!.n, h.inner!.n, h.name!, empty.inner == nil, empty.box == nil)
    h.inner = nil
    print(h.inner == nil)
    return Int32(i.n)
}
