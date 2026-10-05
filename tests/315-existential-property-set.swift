// A settable property requirement assigned through an existential, and
// through a copy of one.
protocol P { var n: Int { get set } }
struct Small: P { var n = 1 }
struct Wide: P { var n = 1, b = 2, c = 3, d = 4, e = 5 }
var s: any P = Small()
s.n = 7
s.n += 1
print(s.n)
var x: any P = Wide()
var y = x
y.n = 50
y.n += 1
print(x.n, y.n)
