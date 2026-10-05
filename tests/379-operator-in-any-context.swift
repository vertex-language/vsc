// An operator whose result goes where an Any is wanted -- a print
// argument -- reads its operands on their own: `"a" + (ok ? "b" : "")`
// is a String sum that then becomes an Any, not a sum of Anys.
let n = 2
let skipped = 3
print("wrote \(n) entries" + (skipped > 0 ? " (\(skipped) skipped)" : ""))
print("a" + (n > 5 ? "b" : "c"))
let x: Any = 10 + (n > 0 ? 1 : 0)
print(x)
let y: Any? = "x" + (n > 0 ? "y" : "z")
print(y!)
print(1.5 * (n > 1 ? 2 : 3))
