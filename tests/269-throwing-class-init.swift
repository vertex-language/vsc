// A class's initializer that throws before its stored properties are all
// set: caught, and with try?. An instance whose initializer threw was
// never made, so its deinit does not run; one that was made is
// deinitialized as usual. A subclass's init throwing through super.init.
enum Bad: Error { case negative(Int), tooBig(Int) }

final class Tensor {
    let shape: [Int]
    let name: String
    init(_ shape: [Int], name: String) throws {
        for d in shape where d < 0 { throw Bad.negative(d) }
        self.shape = shape
        if shape.reduce(1, *) > 100 { throw Bad.tooBig(shape.reduce(1, *)) }
        self.name = name
    }
    deinit { print("deinit", name) }
}

class Base {
    let n: Int
    init(_ n: Int) throws {
        if n == 0 { throw Bad.negative(0) }
        self.n = n
    }
}

final class Derived: Base {
    let label: String
    init(_ n: Int, _ label: String) throws {
        self.label = label
        try super.init(n)
    }
}

func make(_ shape: [Int], _ name: String) {
    do {
        let t = try Tensor(shape, name: name)
        print("made", t.name, t.shape)
    } catch Bad.negative(let d) {
        print("negative", d)
    } catch {
        print("error", error)
    }
}

make([2, 3], "a")
make([2, -1], "b")
make([20, 20], "c")
print((try? Tensor([1], name: "d")) != nil, (try? Tensor([-5], name: "e")) == nil)
let d = try Derived(4, "x")
print(d.n, d.label)
print((try? Derived(0, "y")) == nil)
