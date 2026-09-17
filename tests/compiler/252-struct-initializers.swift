// A struct's declared initializers assign its properties: ones with no
// default, which start with nothing to let go of, and ones whose default
// the body replaces. A generic struct's initializers are lowered for each
// instance they make, and told apart by their labels.
struct Named {
    var first: String
    var last: String
    var note: String = "a default note long enough to live on the heap"

    init(_ f: String, _ l: String) {
        first = f
        last = l
    }

    init(_ f: String, _ l: String, note n: String) {
        first = f
        last = l
        note = n
    }
}

struct Tagged<Value> {
    var value: Value
    var tag: String = "a default tag long enough to live on the heap"
    var uses: Int = 0

    init(_ v: Value) {
        value = v
        uses = 1
    }

    init(_ v: Value, tag t: String) {
        value = v
        tag = t
    }

    func describe() -> Int {
        return tag.count + uses
    }
}

func main() -> Int32 {
    let a = Named("Ada", "Lovelace")
    let b = Named("Grace", "Hopper", note: "n")
    var total = a.first.count + a.last.count + a.note.count
    total += b.first.count + b.last.count + b.note.count
    let t = Tagged(5)
    let u = Tagged("five", tag: "t")
    total += t.value + t.describe() + u.value.count + u.describe()
    return Int32(total % 251)
}
