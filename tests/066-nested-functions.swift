// A function declared inside another, which can see its locals.
func counter(start: Int) -> [Int] {
    var n = start
    func next() -> Int {
        n += 1
        return n
    }
    return [next(), next(), next()]
}
print(counter(start: 10))
