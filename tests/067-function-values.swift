// Functions are values: stored, passed and returned.
func inc(_ x: Int) -> Int { x + 1 }
func dbl(_ x: Int) -> Int { x * 2 }
func apply(_ f: (Int) -> Int, twiceTo x: Int) -> Int { f(f(x)) }
func pick(_ doubling: Bool) -> (Int) -> Int { doubling ? dbl : inc }
let fs: [(Int) -> Int] = [inc, dbl, pick(true), pick(false)]
for f in fs { print(f(10), terminator: " ") }
print()
print(apply(dbl, twiceTo: 5))
