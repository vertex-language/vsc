// A static method of a generic type is called on an instance's type, and
// is lowered for it: its body's `Stack<Element>` is that instance. Locals
// inside generic code are written with the parameters, and have the
// arguments' types wherever the code is lowered for them.
struct Stack<Element> {
    var items: [Element] = []

    static func of(_ x: Element) -> Stack<Element> {
        var s = Stack<Element>()
        s.items.append(x)
        return s
    }

    static func pair(_ a: Element, _ b: Element) -> Stack<Element> {
        var s = of(a)
        s.items.append(b)
        return s
    }
}

func collect<T>(_ a: T, _ b: T) -> [T] {
    var acc: [T] = []
    acc.append(a)
    let second: T = b
    acc.append(second)
    return acc
}

func main() -> Int32 {
    let s = Stack<Int>.of(7)
    let words = Stack<String>.pair("ab", "a string long enough to live on the heap")
    let ints = collect(4, 5)
    let strings = collect("x", "yz")
    var total = s.items.count + s.items[0] + words.items.count + words.items[1].count
    total += ints[0] + ints[1] + strings.count + strings[1].count
    return Int32(total % 251)
}
