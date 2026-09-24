// class methods can be overridden; static and final ones cannot.
class Base {
    class func kind() -> String { "base" }
    static func shared() -> String { "shared" }
    final func id() -> String { "id of \(Self.kind())" }
    func who() -> String { type(of: self).kind() }
}
final class Derived: Base {
    override class func kind() -> String { "derived" }
}
print(Base.kind(), Derived.kind(), Derived.shared())
let objs: [Base] = [Base(), Derived()]
for o in objs { print(o.who(), o.id()) }
