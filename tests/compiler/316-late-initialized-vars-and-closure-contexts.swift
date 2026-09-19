// A var declared without a value and given one later -- on every path, on
// some, in a loop -- is destroyed at the end of its scope like any other:
// the deinits below say when. And a closure that cannot escape, called in
// place the way withUnsafeBytes calls its body, lets go of what it
// captured exactly once, in a plain function and across an await.
@_silgen_name("strnlen") func cstrnlen(_ s: UnsafeRawPointer, _ n: Int) -> Int

final class Tag {
    let name: String
    init(_ name: String) { self.name = name; print("init \(name)") }
    deinit { print("deinit \(name)") }
}

struct Holder {
    var tag: Tag
    var bytes: [UInt8]
}

func pick(_ c: Bool) -> Int {
    var h: Holder
    if c {
        h = Holder(tag: Tag("yes"), bytes: [1, 2])
    } else {
        h = Holder(tag: Tag("no"), bytes: [3])
    }
    print("picked \(h.tag.name)")
    return h.bytes.count
}

func loop() -> Int {
    var total = 0
    for i in 0..<3 {
        var t: Tag
        t = Tag("loop\(i)")
        total += t.name.count
    }
    return total
}

func maybe(_ c: Bool) -> Int {
    var t: Tag?
    if c {
        t = Tag("maybe")
    }
    return t == nil ? 0 : 1
}

func useTag(_ a: [UInt8], _ t: Tag, _ s: String) -> Int {
    return a.withUnsafeBytes { p in
        cstrnlen(p.baseAddress!, 8) + t.name.count + s.count
    }
}

func across(_ a: [UInt8], _ t: Tag) async -> Int {
    let n = a.withUnsafeBytes { p in cstrnlen(p.baseAddress!, 8) + t.name.count }
    await Task.yield()
    let m = a.withUnsafeBytes { p in cstrnlen(p.baseAddress!, 8) + t.name.count }
    return n + m
}

func main() async -> Int32 {
    print(pick(true), pick(false))
    print(loop())
    print(maybe(true), maybe(false))
    let a: [UInt8] = [1, 2, 0]
    var total = 0
    for i in 0..<2 {
        let t = Tag("t\(i)")
        total += useTag(a, t, "a string longer than fifteen bytes \(i)")
        total += await across(a, t)
    }
    print("total \(total)")
    return Int32(total % 50)
}
