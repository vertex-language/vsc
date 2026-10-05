// switch over an Optional: .some and .none, the ? pattern, and nested optionals.
func describe(_ x: Int?) -> String {
    switch x {
    case nil: return "nothing"
    case 0?: return "zero"
    case let n? where n < 0: return "negative \(n)"
    case .some(let n): return "positive \(n)"
    }
}
print([nil, 0, -3, 8].map(describe))
let pair: (Int?, String?) = (1, nil)
switch pair {
case (let a?, let b?): print("both", a, b)
case (let a?, nil): print("left only", a)
case (nil, _): print("no left")
}
