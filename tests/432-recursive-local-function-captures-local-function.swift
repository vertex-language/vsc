// A recursive local function that throws and calls another local
// function, both capturing a local sink.
indirect enum Node {
    case file(String)
    case dir(String, [Node])
}
struct Full: Error {}
func format(_ root: Node, limit: Int) throws -> [String] {
    var f: [String] = []
    var next = 1
    func writeInode(_ name: String, parent: Int) throws -> Int {
        if f.count >= limit { throw Full() }
        let n = next
        next += 1
        f.append("\(n)<-\(parent) \(name)")
        return n
    }
    func write(_ n: Node, parent: Int) throws {
        switch n {
        case .file(let name):
            _ = try writeInode(name, parent: parent)
        case .dir(let name, let kids):
            let me = try writeInode(name, parent: parent)
            for k in kids { try write(k, parent: me) }
        }
    }
    try write(root, parent: 0)
    return f
}
let tree = Node.dir("/", [.file("a"), .dir("etc", [.file("hosts")]), .file("b")])
print(try format(tree, limit: 10))
print((try? format(tree, limit: 2)) == nil)
