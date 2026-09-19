// A block holding many assignments of non-trivial values: an init that
// sets every stored property, a run of `+=` on strings, and writes into
// a nested struct's properties. Each one expands in place when it is
// resolved, and every one of them must be.

struct Desc {
    var type: String
    var sdp: String
}

struct Agent {
    var ufrag = ""
    var pwd = ""
    var names: [String] = []
}

struct Conn {
    var a: String
    var b: String
    var c: String
    var d: Desc
    var e: [Int]
    var agent: Agent
    var f: String
    var count: UInt64 = 0

    init(name: String) {
        self.a = name
        self.b = ""
        self.c = name + "!"
        self.d = Desc(type: "offer", sdp: "")
        self.e = [1, 2, 3]
        self.agent = Agent()
        self.f = "last"
        self.count = 7
    }

    mutating func accept(_ remote: Desc) {
        self.agent.ufrag = remote.type
        self.agent.pwd = remote.sdp
        self.agent.names = [remote.type, remote.sdp]
        self.d.sdp = remote.sdp
        self.f = remote.type + remote.sdp
    }
}

func build() -> String {
    var out = ""
    out += "v=0\n"
    out += "o=- 1 1 IN IP4 0.0.0.0\n"
    out += "s=-\n"
    out += "t=0 0\n"
    out += "a=ice-ufrag:u\n"
    out += "a=ice-pwd:p\n"
    return out
}

func main() -> Int32 {
    var c = Conn(name: "x")
    print(c.a, c.b.count, c.c, c.d.type, c.e.count, c.f, c.count)
    c.accept(Desc(type: "answer", sdp: "v=0"))
    var outer = Conn(name: "y")
    outer.agent.ufrag = c.agent.ufrag
    outer.agent.pwd = c.agent.pwd
    outer.d.type = c.d.sdp
    print(c.agent.ufrag, c.agent.pwd, c.agent.names.count, c.d.sdp, c.f)
    print(outer.agent.ufrag, outer.agent.pwd, outer.d.type)
    let s = build()
    print(s.count)
    return Int32(s.count + c.f.count)
}
