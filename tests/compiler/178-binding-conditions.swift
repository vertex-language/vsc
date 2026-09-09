// Two bindings in one guard, the second reading the first.
func f(_ a: Int32?, _ b: Int32?) -> Int32 {
    guard let x = a, let y = b, x < y else { return 0 }
    return y - x
}
func main() -> Int32 {
    return f(2, 9) + f(9, 2) + f(nil, 9) + f(2, nil)
}
