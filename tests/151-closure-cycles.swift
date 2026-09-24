// A closure that captures self strongly keeps it alive; [weak self] does not.
class Timer {
    let name: String
    var onFire: (() -> Void)?
    init(_ n: String) { name = n }
    func armStrong() { onFire = { print("fire", self.name) } }
    func armWeak() { onFire = { [weak self] in print("fire", self?.name ?? "nil") } }
    deinit { print("deinit", name) }
}
var a: Timer? = Timer("strong")
a!.armStrong()
a!.onFire!()
a = nil
print("strong not freed")
var b: Timer? = Timer("weak")
b!.armWeak()
let f = b!.onFire!
f()
b = nil
f()
