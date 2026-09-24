// A generic struct and a generic class, used at several types.
struct Stack<Element> {
    private var items: [Element] = []
    mutating func push(_ x: Element) { items.append(x) }
    mutating func pop() -> Element? { items.popLast() }
    var top: Element? { items.last }
    var count: Int { items.count }
}
final class Ref<T> {
    var value: T
    init(_ v: T) { value = v }
}
var ints = Stack<Int>()
ints.push(1); ints.push(2)
var strs = Stack<String>()
strs.push("x")
print(ints.pop() as Any, ints.top as Any, ints.count, strs.top as Any)
let r = Ref([1, 2])
r.value.append(3)
print(r.value, Ref("s").value)
