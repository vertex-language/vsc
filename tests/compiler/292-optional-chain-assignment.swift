// An assignment whose destination is an optional chain is the chain:
// where a step finds nil, nothing is assigned and the value is never
// evaluated. Plain and compound assignments, a counted value, two steps,
// and reads through chains that stop at their second or third step, with
// what the earlier steps held let go of on the way.

final class Node {
    var v = 0
    var name = ""
    var next: Node? = nil
}
struct Box { var n: Int }
final class Holder { var box: Box? = Box(n: 1) }
var evaluated = 0
func value(_ n: Int) -> Int { evaluated += 1; return n }
final class N { var v = 1; var next: N? = nil }

func main() -> Int32 {
    var total = 0
    var t = 0
    for i in 0..<500 {
        let a = Node()
        a.next?.v = value(5)
        a.next = Node()
        a.next?.v = value(i)
        a.next?.v += 2
        a.next?.name = "n" + String(i)
        a.next?.next?.v = value(9)
        total += a.next!.v + a.next!.name.count
        let b = N()
        if i % 2 == 0 { b.next = N() }
        if i % 4 == 0 { b.next?.next = N() }
        t += b.next?.next?.v ?? 5
        t += (b.next?.v ?? 0) + (b.next?.next?.next?.v ?? 7)
    }
    print(total, evaluated, t)
    return Int32((total + t) % 256)
}
