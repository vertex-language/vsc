// Two names for one instance see the same writes.
final class Cell {
    var n: Int32 = 0
}

func bump(_ c: Cell) { c.n = c.n + 1 }

func main() -> Int32 {
    let a = Cell()
    let b = a
    bump(a)
    bump(b)
    bump(a)
    return a.n * 10 + b.n
}
