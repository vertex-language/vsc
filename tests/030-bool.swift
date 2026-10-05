// Bool: its literals, toggle, and equality.
var flag = false
print(flag)
flag.toggle()
print(flag, flag == true, flag != false)
let both = flag && !flag
print(both, Bool("true") as Any, Bool("yes") as Any)
