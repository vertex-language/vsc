// ! on nil traps.
var total = 0
for v in ["1", "two"] {
    total += Int(v)!
}
