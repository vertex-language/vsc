// inout reaches through properties, computed properties and subscripts, and writes back.
struct Box {
    var stored = 1
    var computed: Int {
        get { print("get"); return stored * 10 }
        set { print("set", newValue); stored = newValue / 10 }
    }
}
class Holder { var box = Box() }
func triple(_ x: inout Int) { x *= 3 }
var b = Box()
triple(&b.stored)
triple(&b.computed)
let h = Holder()
triple(&h.box.stored)
var dict = ["k": 2]
triple(&dict["k", default: 0])
print(b.stored, h.box.stored, dict["k"]!)
