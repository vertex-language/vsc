// An array and a dictionary boxed as Any by generic code, cast back with
// as? to their own type and not to another.
func box<T>(_ v: T) -> Any { v }
let a = box(["a", "b"])
if let s = a as? [String] { print(s) }
print(a as? [Int] as Any)
let d = box(["k": 1])
if let m = d as? [String: Int] { print(m["k"]!) }
let n = box(3)
print(n as? Int ?? -1, n as? [Int] as Any)
