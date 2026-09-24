// A capture list copies the value when the closure is made.
var x = 1
let byReference = { x }
let byValue = { [x] in x }
let renamed = { [y = x * 100] in y }
x = 2
print(byReference(), byValue(), renamed())
