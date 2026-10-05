// A value wider than an existential's inline buffer is boxed: five and
// eight words, held as `any P`, copied, and called through.
protocol Summed { func sum() -> Int }
struct Five: Summed { var a = 1, b = 2, c = 3, d = 4, e = 5
    func sum() -> Int { a + b + c + d + e } }
struct Eight: Summed { var a = 1, b = 2, c = 3, d = 4, e = 5, f = 6, g = 7, h = 8
    func sum() -> Int { a + b + c + d + e + f + g + h } }
var xs: [any Summed] = [Five(), Eight()]
var copy = xs
copy[0] = Eight()
for x in xs { print(x.sum()) }
for x in copy { print(x.sum()) }
let one: any Summed = Eight(a: 100)
print(one.sum())
