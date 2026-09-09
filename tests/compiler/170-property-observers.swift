// willSet and didSet. They were parsed and dropped, so a write stored
// the value and ran neither -- a program that compiled, ran, and did
// half of what its source says, with nothing reported at any point.
//
// A write to such a property is the store with the observers around
// it, and the old value is read before the store because after it
// there is nothing left to read:
//
//     old = self.n
//     willSet(new)
//     self.n = new
//     didSet(old)
struct Recorder {
    var log: Int32 = 0

    var both: Int32 = 0 {
        willSet { log = log * 10 + 1 }
        didSet { log = log * 10 + 2 }
    }

    // The value each observer is handed, under its own implicit name.
    var incoming: Int32 = 0 {
        willSet { log = log * 10 + newValue }
    }
    var outgoing: Int32 = 7 {
        didSet { log = log * 10 + oldValue }
    }

    // And under a name the source chose.
    var named: Int32 = 0 {
        willSet(nv) { log = log * 10 + nv }
        didSet(ov) { log = log * 10 + ov }
    }
}

class Watched {
    var seen: Int32 = 0
    var n: Int32 = 0 {
        didSet { seen = oldValue + 7 }
    }
}

func main() -> Int32 {
    var r = Recorder()
    r.both = 1        // log 12
    r.incoming = 3    // log 123
    r.outgoing = 9    // didSet sees the old 7: log 1237
    r.named = 4       // willSet 4, didSet 0: log 123740

    // A compound assignment reads the storage and writes through the
    // observers.
    var c = Recorder()
    c.both += 5       // willSet then didSet: log 12

    let w = Watched()
    w.n = 3           // oldValue 0

    return (r.log % 1000) + c.log + w.seen
}
