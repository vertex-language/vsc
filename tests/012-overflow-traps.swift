// Overflow on + traps. A trap ends the program by a signal, and output
// buffered before it may be lost, so a trapping rung prints nothing.
var x = Int.max - 2
for _ in 0..<5 {
    x += 1
}
