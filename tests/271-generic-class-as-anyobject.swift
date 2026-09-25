// A generic class's instance, and a plain class's, stored as AnyObject and
// cast back: AnyObject requires nothing of either.
final class Box<T> { let v: T; init(_ v: T) { self.v = v } }
final class Plain { let n = 3 }
final class Holder { var owner: AnyObject?; init() {} }
let h = Holder()
h.owner = Plain()
print(h.owner is Plain)
h.owner = Box(2.5)
print(h.owner is Box<Double>, (h.owner as? Box<Double>)?.v ?? 0)
let objs: [AnyObject] = [Box(1), Plain()]
print(objs.count)
