// A class instance stored as AnyObject? and cast back down.
class Cell { let id: Int; init(_ id: Int) { self.id = id } }
final class Box: Cell {}
final class Holder { var target: AnyObject? }
let h = Holder()
h.target = Box(4)
if let b = h.target as? Box { print("box", b.id) }
if let c = h.target as? Cell { print("cell", c.id) }
print(h.target is Holder)
h.target = nil
print(h.target as? Cell == nil)
