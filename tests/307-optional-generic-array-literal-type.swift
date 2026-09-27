// An array type of an optional generic, `[Box<UInt8>?]`, in an expression:
// `>?` is two tokens there.
struct Box<T> { var v: T }
let xs = [Box<UInt8>?](repeating: nil, count: 3)
print(xs.count, xs[0] == nil)
var ys = [Box<Int>?]()
ys.append(Box(v: 1))
print(ys[0]!.v)
