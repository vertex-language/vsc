// Replacing a stored reference releases the old one, and a name still holding it keeps it alive.
final class Leaf { var n: Int32 = 7 }
final class Tree { var leaf = Leaf() }
func main() -> Int32 {
  let t = Tree(); let keep = t.leaf
  t.leaf = Leaf(); t.leaf.n = 35
  return keep.n + t.leaf.n
}
