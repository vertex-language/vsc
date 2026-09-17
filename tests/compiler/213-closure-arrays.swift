// An array may hold functions: literal closures, closures that capture,
// declared functions. Copying the array shares every context, appending
// and replacing let the old ones go, and an element is called in place.
func triple(_ x: Int) -> Int {
    return x * 3
}

func main() -> Int32 {
    let base = 10
    var counter = 0
    var table: [(Int) -> Int] = [{ $0 + 1 }, { $0 * 2 }, { $0 + base }]
    var total = 0
    for f in table {
        total += f(5)
    }

    let snapshot = table
    table.append({ x in
        counter += 1
        return x * x
    })
    table.append(triple)
    for f in table {
        total += f(3)
    }
    total += snapshot.count * 100

    table[0] = { $0 - base }
    total += table[0](50) + snapshot[0](50)
    total += table[3](4) + (table[4])(2)

    var none: [() -> Int] = []
    none.append({ counter })
    total += none[0]() * 1000
    return Int32(total % 251)
}
