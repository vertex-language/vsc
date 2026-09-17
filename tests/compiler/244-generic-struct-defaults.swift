// An instance of a generic struct takes its properties' defaults from the
// declaration, with the declaration's type parameters standing for the
// instance's arguments: an empty [Element], a nil B?, and values of plain
// types beside them. Properties passed explicitly still override them.
struct Stack<Element> {
    var items: [Element] = []
    var limit: Int = 8
}

struct Pair<A, B> {
    var first: [A] = []
    var second: B? = nil
    var name: String = "pair"
}

func main() -> Int32 {
    var s = Stack<Int>()
    s.items.append(3)
    s.items.append(9)
    var words = Stack<String>()
    words.items.append("a string long enough to live on the heap")
    var p = Pair<String, Int>()
    p.first.append("x")
    p.second = 4
    let q = Stack<Double>(items: [1.5], limit: 2)
    var total = s.items.count + s.items[1] + s.limit + words.items[0].count
    total += p.first.count + (p.second ?? 0) + p.name.count
    total += q.items.count + q.limit
    return Int32(total)
}
