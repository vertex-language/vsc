// Closures held in an array of tuples, called from a loop.
enum Act { case relu, neg }
let table: [(Act, (Double) -> Double, Int)] = [
    (.relu, { max(0, $0) }, 1),
    (.neg, { -$0 }, 2),
]
for (a, f, n) in table { print(a, f(-1.5), f(2), n) }
