// Local functions that call each other, over an indirect enum.
indirect enum Node { case leaf(Int), pair(Node, Node), neg(Node) }
func total(_ root: Node) -> Int {
    func walk(_ n: Node) -> Int {
        switch n {
        case .leaf(let v): return v
        case .pair(let a, let b): return walk(a) + walk(b)
        case .neg(let x): return negate(x)
        }
    }
    func negate(_ n: Node) -> Int { -walk(n) }
    return walk(root)
}
print(total(.pair(.leaf(3), .neg(.pair(.leaf(1), .leaf(10))))))
