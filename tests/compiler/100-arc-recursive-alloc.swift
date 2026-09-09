// An allocation per frame, released as each frame ends.
final class Node { var n: Int32 = 0 }
func build(_ depth: Int32) -> Int32 {
  if depth == 0 { return 0 }
  let n = Node(); n.n = depth
  return n.n + build(depth - 1)
}
func main() -> Int32 { return build(8) + 6 }
