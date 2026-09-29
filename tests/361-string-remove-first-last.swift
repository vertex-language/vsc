// String's removeFirst and removeLast.
var s = "\"quoted\""
let open = s.removeFirst()
let close = s.removeLast()
s.removeFirst(2)
s.removeLast(1)
print(open, close, s)
