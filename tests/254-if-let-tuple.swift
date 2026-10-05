// if let binding a tuple out of an optional.
func divide(_ a: Int, _ b: Int) -> (Int, Int)? {
    if b == 0 { return nil }
    return (a / b, a % b)
}
if let (q, r) = divide(17, 5) { print(q, r) }
if let (q, r) = divide(1, 0) { print(q, r) } else { print("none") }
