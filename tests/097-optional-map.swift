// map and flatMap on an Optional, and optional comparison with ==.
let a: Int? = 4
let b: Int? = nil
print(a.map { $0 * 2 } as Any, b.map { $0 * 2 } as Any)
print(a.flatMap { $0 > 3 ? String($0) : nil } as Any)
print(a == 4, b == nil, a != b, Optional(3) == Optional(3))
let nested: Int?? = .some(nil)
print(nested as Any, nested == nil)
