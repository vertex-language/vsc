// A struct held in a var can have a field written.
struct Counter {
    var value: Int32
}

func main() -> Int32 {
    var c = Counter(value: 1)
    c.value = c.value + 10
    c.value = c.value * 2
    return c.value
}
