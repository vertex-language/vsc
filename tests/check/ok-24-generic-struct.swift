struct Wrapper<T> {
    var value: T
}
func use() -> Int {
    let w = Wrapper(value: 3)
    return w.value
}
