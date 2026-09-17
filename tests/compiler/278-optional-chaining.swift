// Optional chaining the whole way: members, method calls and subscripts
// after an `a?`, several `?` in one chain, a result already optional (which
// is not wrapped again), a chain that ends in a call answering nothing, and
// optionals of structs that own references -- laid out, as Swift does, in
// the struct's own words with nil a reference none of them holds.
final class Node {
    let value: Int
    var next: Node?
    init(_ value: Int, next: Node? = nil) { self.value = value; self.next = next }
    func describe() -> String { return "node \(value)" }
}

struct Wide {
    let label: String
    let tags: [Int]
    let child: Wide2?
    func size() -> Int { return tags.count }
    func first() -> Int? { return tags.isEmpty ? nil : tags[0] }
}

struct Wide2 {
    let name: String
}

var log: [String] = []

struct Logger {
    func note(_ s: String) { log.append(s) }
}

func find(_ ok: Bool) -> Wide? {
    return ok ? Wide(label: "w", tags: [4, 5, 6], child: Wide2(name: "kid")) : nil
}

func main() -> Int32 {
    var failures: Int32 = 0

    let w = find(true)
    let none = find(false)
    if w?.label != "w" || none?.label != nil { failures += 1 }
    if w?.tags.count != 3 || none?.tags.count != nil { failures += 1 }
    if w?.size() != 3 || w?.tags[1] != 5 { failures += 1 }
    if w?.child?.name != "kid" || none?.child?.name != nil { failures += 1 }
    // first() answers Int? already, so the chain is an Int?, not an Int??.
    let f: Int? = w?.first()
    if f != 4 { failures += 1 }

    let list = Node(1, next: Node(2, next: Node(3)))
    if list.next?.next?.value != 3 || list.next?.next?.next?.value != nil { failures += 1 }
    if list.next?.describe() != "node 2" { failures += 1 }

    let logger: Logger? = Logger()
    logger?.note("one")
    let missing: Logger? = nil
    missing?.note("two")
    if log != ["one"] { failures += 1 }

    let opt: Wide? = Wide(label: "x", tags: [], child: nil)
    if let got = opt, got.label != "x" { failures += 1 }
    if opt?.child?.name != nil || opt?.first() != nil { failures += 1 }

    print(w?.label ?? "-", none?.label ?? "-", w?.child?.name ?? "-", list.next?.describe() ?? "-")
    print(failures == 0 ? "ok" : "failed \(failures)")
    return failures
}
