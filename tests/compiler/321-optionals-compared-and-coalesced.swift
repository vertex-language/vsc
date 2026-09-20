// An optional compared with a value read through a borrow -- a class's
// field -- on one arm of the comparison, which has to give the borrow
// back on that arm; and `??` whose fallback is a struct too wide for
// registers, answered by address and handed to the join as its fields.

final class Node {
    let id: Int64
    init(_ id: Int64) { self.id = id }
}

struct Config {
    var name: String
    var color: UInt32
    var loader: ((String) -> [UInt8]?)?
    init(name: String = "default", color: UInt32 = 1) {
        self.name = name
        self.color = color
        loader = nil
    }
}

final class Holder {
    var focused: Node?
    var config: Config
    init(configuration: Config?, focused: Node?) {
        config = configuration ?? Config()
        self.focused = focused
    }
}

func main() -> Int32 {
    let a = Node(1)
    let b = Node(2)
    let h = Holder(configuration: nil, focused: a)
    print(h.focused?.id == a.id, h.focused?.id == b.id, h.focused?.id != b.id)
    let none = Holder(configuration: Config(name: "given", color: 7), focused: nil)
    print(none.focused?.id == a.id, none.focused?.id != a.id)
    print(h.config.name, h.config.color, none.config.name, none.config.color)
    let x: Int? = 3
    let y: Int? = 3
    let z: Int? = nil
    print(x == y, x == z, z == z, x == Int(a.id) + 2)
    let widths: [Config?] = [nil, Config(name: "w")]
    for w in widths {
        let c = w ?? Config(name: "fallback", color: 9)
        print(c.name, c.color)
    }
    return 0
}
