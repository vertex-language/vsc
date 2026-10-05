// A mutating call through a computed property is written back when its
// statement ends, even where an argument's getter is specialized for the
// first time in the middle of that statement.
@propertyWrapper
struct Box<T> {
    var v: T
    init(wrappedValue: T) { v = wrappedValue }
    var wrappedValue: T {
        get { return v }
        set { v = newValue }
    }
}

struct Bag<T> {
    var items: [T] = []
    var count: Int { items.count }
}

func run() {
    @Box var xs: [String] = []
    @Box var s = "a"
    xs.append(s)
    xs.append(s + "b")
    print(xs)

    @Box var bag = Bag<Double>()
    @Box var d = 1.5
    bag.items.append(d)
    print(bag.count, bag.items)
}
run()
