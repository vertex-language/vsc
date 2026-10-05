struct Stack<Element> {
    var items: [Element] = []
    mutating func push(_ item: Element) { items.append(item) }
}
func use() {
    var s = Stack<Int>()
    s.push(1)
}
