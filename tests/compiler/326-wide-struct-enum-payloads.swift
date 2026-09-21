// A struct too wide for registers -- held in memory -- can still be the
// payload of an enum case: its scalars are read out of its storage and
// packed into the enum's words, the same as a struct that arrived in
// registers.

struct Cursor {
    var width: Int
    var height: Int
    var hotX: Int
    var hotY: Int
    var pixels: [UInt8]
}

struct Wide {
    var a: Int
    var b: Int
    var c: Int
    var d: Int
    var e: Int
    var f: Int
}

enum Event {
    case frame(Int)
    case pointer(Cursor)
    case wide(Wide)
    case hidden
}

func makeCursor(_ n: Int) -> Cursor {
    var px: [UInt8] = []
    var i = 0
    while i < n { px.append(UInt8(i)); i += 1 }
    return Cursor(width: n, height: n + 1, hotX: 2, hotY: 3, pixels: px)
}

func makeWide() -> Wide {
    return Wide(a: 1, b: 2, c: 3, d: 4, e: 5, f: 6)
}

func describe(_ e: Event) -> String {
    switch e {
    case .frame(let n): return "frame \(n)"
    case .pointer(let c): return "pointer \(c.width)x\(c.height) hot \(c.hotX),\(c.hotY) \(c.pixels.count) bytes last \(c.pixels[c.pixels.count - 1])"
    case .wide(let w): return "wide \(w.a + w.b + w.c + w.d + w.e + w.f)"
    case .hidden: return "hidden"
    }
}

var pending: [Event] = []

func push(_ c: Cursor) {
    let e: Event = .pointer(c)
    pending.append(e)
}

func main() -> Int32 {
    let c = makeCursor(4)
    push(c)
    pending.append(.pointer(makeCursor(7)))
    pending.append(.wide(makeWide()))
    pending.append(.hidden)
    pending.append(.frame(9))
    for e in pending {
        print(describe(e))
    }
    // The original is untouched by being carried.
    print(c.pixels.count, c.hotY)
    return 0
}
