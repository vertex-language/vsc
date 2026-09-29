// One existential into another of fewer protocols: `any Q` and `any Error`
// into `Any`, and `any P & Q` into `any Q`. The value, its type and the
// tables the destination names come across; the source's other tables do
// not, because the destination has no room for them.
protocol P { func p() -> Int }
protocol Q { func q() -> Int }
struct S: P, Q {
    var x: Int
    func p() -> Int { x + 1 }
    func q() -> Int { x + 2 }
}
enum Bad: Error { case negative(Int) }

let q: any Q = S(x: 3)
let a: Any = q
print(a)

let e: any Error = Bad.negative(4)
let b: Any = e
print(b)
print("caught", e)

let pq: any P & Q = S(x: 10)
let justQ: any Q = pq
print(justQ.q())

var box: Any = 0
box = pq
print(box)
