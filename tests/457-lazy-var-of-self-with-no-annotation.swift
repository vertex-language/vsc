// A lazy property with no type written, whose value names self.
final class Screen {
    let n: Int
    init(owner: Owner) { n = owner.k }
}
final class Owner {
    var k = 5
    lazy var screen = Screen(owner: self)
    lazy var twice = self.k * 2
    lazy var later = Later(owner: self)
}
struct Later {
    let k: Int
    init(owner: Owner) { k = owner.k + 1 }
}
struct Box {
    var k = 2
    lazy var doubled = self.k * 2
}
let o = Owner()
print(o.screen.n, o.twice, o.later.k)
o.k = 100
print(o.screen.n, o.twice)
var b = Box()
print(b.doubled)
