func apply(_ f: (Int) -> Int, to n: Int) -> Int {
    return f(n)
}
func use() -> Int {
    let double = { (x: Int) -> Int in x * 2 }
    return apply(double, to: 3)
}
