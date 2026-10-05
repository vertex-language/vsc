// A property wrapper on a local: its storage is a local of the wrapper's
// type, the name reads and writes through it, and $name is its projection.
@propertyWrapper
struct Clamped {
    private var value: Int
    var wrappedValue: Int {
        get { value }
        set { value = min(newValue, 10) }
    }
    var projectedValue: Int { value * 100 }
    init(wrappedValue: Int) { value = min(wrappedValue, 10) }
}

@propertyWrapper
final class Cell<T> {
    var wrappedValue: T
    init(wrappedValue: T) { self.wrappedValue = wrappedValue }
    var projectedValue: Cell<T> { self }
}

func bump(_ c: Cell<Int>) { c.wrappedValue += 5 }

func run() {
    @Clamped var count = 50
    print(count)
    count = 3
    count += 1
    print(count, $count)
    let add = { count += 100 }
    add()
    print(count)

    @Cell var draft = "hi"
    draft += "!"
    print(draft)

    @Cell var n = 1
    bump($n)
    print(n)
    let twice = { n * 2 }
    n = 7
    print(twice())

    // The type written is the wrapped value's: an empty literal is read as it.
    @Cell var xs: [Int] = []
    xs.append(4)
    xs += [5]
    print(xs, $xs.wrappedValue.count)
}
run()
