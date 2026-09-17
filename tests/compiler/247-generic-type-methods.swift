// A generic type's methods are called on its instances: a mutating one, one
// returning an optional of the type's parameter, one calling another on
// self, and one declared in an extension, for instances of an Int and a
// String stack.
struct Stack<Element> {
    var items: [Element] = []

    mutating func push(_ x: Element) {
        items.append(x)
    }

    func peek() -> Element? {
        return items.last
    }

    func top() -> Element? {
        return peek()
    }

    func size() -> Int {
        return items.count
    }
}

extension Stack {
    func isEmptyStack() -> Bool {
        return size() == 0
    }
}

func main() -> Int32 {
    var ints = Stack<Int>()
    ints.push(3)
    ints.push(9)
    var words = Stack<String>()
    words.push("a string long enough to live on the heap")
    var total = (ints.peek() ?? 0) + (ints.top() ?? 0) + ints.size()
    total += (words.top() ?? "").count + words.size()
    if !words.isEmptyStack() {
        total += 100
    }
    let empty = Stack<Double>()
    if empty.isEmptyStack() {
        total += 1
    }
    return Int32(total % 251)
}
