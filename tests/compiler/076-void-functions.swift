// A function that returns nothing still runs.
final class Cell {
    var n: Int32 = 0
}

func bump(_ c: Cell) {
    c.n = c.n + 1
}

func bumpTwice(_ c: Cell) {
    bump(c)
    bump(c)
}

func main() -> Int32 {
    let c = Cell()
    bump(c)
    bumpTwice(c)
    return c.n * 14
}
