// A var that holds a reference, reassigned.
final class Box {
    var n: Int32 = 0
}

func main() -> Int32 {
    var b = Box()
    b.n = 10
    let first = b
    b = Box()
    b.n = 32
    return first.n + b.n
}
