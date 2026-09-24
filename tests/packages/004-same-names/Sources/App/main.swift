import Screen
import Pointer

var w = Window()
w.setCursor(.hidden)
print(describe(w.cursor), describe(.arrow))

let c = Pointer.Cursor(width: 3).doubled()
print(c.width)
