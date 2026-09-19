// Optionals of structs the runtime has to describe: the result of a
// dictionary lookup, an array's popLast, and an array literal of T? whose
// elements are wrapped. A struct with a String or an array in it spares a
// word that is never zero, which is where its nil lives.

enum Kind {
    case udp
    case tcp(Int)
}

struct Allocation {
    var relay: String
    var kind: Kind
    var lifetime: Int
    var perms: [String]
}

struct Plain {
    var a: Int
    var b: Int
}

struct Session {
    var user: String
    var alloc: Allocation?
}

func main() -> Int32 {
    var bytes: [UInt8] = [1, 2, 3]
    var total = 0
    while let b = bytes.popLast() {
        total += Int(b)
    }
    print(total, bytes.count, bytes.popLast() == nil)

    var names = ["x", "yy", "zzz"]
    let before = names
    if let last = names.popLast() {
        print(last, names.count, before.count)
    }

    var table: [String: Allocation] = [:]
    table["a"] = Allocation(relay: "r1", kind: .tcp(3), lifetime: 600, perms: ["x"])
    let hit = table["a"]
    let miss = table["b"]
    print(hit?.relay ?? "-", hit?.lifetime ?? 0, miss == nil)

    var plains: [Plain?] = [Plain(a: 1, b: 2), nil]
    plains.append(Plain(a: 3, b: 4))
    let p = Plain(a: 7, b: 8)
    let more: [Plain?] = [p, nil, p]
    print(plains.count, plains[1] == nil, plains[2]?.b ?? 0, more.count, more[2]?.a ?? 0)

    var sessions: [String: Session] = [:]
    sessions["alice"] = Session(user: "alice", alloc: hit)
    if let s = sessions["alice"], let al = s.alloc {
        print(s.user, al.relay, al.perms.count)
    }

    var sessionsList = [Session(user: "a", alloc: nil), Session(user: "b", alloc: hit)]
    let lastSession = sessionsList.popLast()
    print(lastSession?.user ?? "-", lastSession?.alloc?.relay ?? "-", sessionsList.count)
    return Int32(total)
}
