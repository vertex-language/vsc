// A struct is copied on assignment and on passing.
struct Box { var value: Int }
var a = Box(value: 1)
var b = a
b.value = 2
func bump(_ x: Box) -> Box {
    var y = x
    y.value += 100
    return y
}
print(a.value, b.value, bump(a).value, a.value)
let fixed = Box(value: 9)
print(fixed.value)
