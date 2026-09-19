// Writing through an array element: a stored property of one, a mutating
// method on one, and an element of an element. The array's storage is
// made its variable's alone and the element changed where it is, so a
// copy taken before is untouched -- and the value written, or the
// arguments to the method, are evaluated first, so one that reads or
// copies the array sees it as it was.

struct Inner { var v = 0 }
struct Item {
    var name: String
    var tags: [String] = []
    var inner = Inner()
    var count = 0
    mutating func bump(_ by: Int) { count += by }
    mutating func absorb(_ all: [Item]) { count += all.count; tags.append(String(all[0].count)) }
}
final class Box { var items: [Item] = [] }

func main() -> Int32 {
    var a = [Item(name: "x"), Item(name: "y")]
    let before = a
    a[0].name = "z"
    a[1].tags.append("t")
    a[1].tags.append("u")
    a[0].inner.v = 7
    a[0].count += 5
    a[1].name += "!"
    var i = 1
    a[i].bump(2)
    a[i].bump(a[0].count)
    a[0].absorb(a)
    print(a[0].name, a[0].count, a[0].inner.v, a[0].tags, a[1].name, a[1].tags, a[1].count)
    print(before[0].name, before[0].count, before[1].tags.count)

    var grid: [[Int]] = [[], [1]]
    for k in 0..<5 { grid[k % 2].append(k) }
    grid[1][0] = 9
    grid[0][1] += 10
    let snapshot = grid
    grid[0][0] = -1
    print(grid, snapshot)

    var nested: [[Item]] = [[Item(name: "n")]]
    nested[0][0].tags.append("deep")
    nested[0][0].inner.v += 3
    print(nested[0][0].tags, nested[0][0].inner.v)

    let box = Box()
    box.items.append(Item(name: "b"))
    box.items[0].count = 4
    box.items[0].bump(1)
    print(box.items[0].count)

    var total = 0
    var big = [Int](repeating: 0, count: 5000)
    var cells = [Inner](repeating: Inner(), count: 5000)
    for k in 0..<5000 { big[k] += k % 3; cells[k].v += 1; i = k }
    for k in 0..<5000 { total += big[k] + cells[k].v }
    print(total, i)
    return Int32(a[0].count)
}
