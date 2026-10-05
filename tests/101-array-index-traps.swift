// Reading past the end of an array traps.
let xs = [1, 2, 3]
var total = 0
for i in 0...3 {
    total += xs[i]
}
