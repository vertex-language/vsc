// §7 / §6.2 Composed Property Wrappers, Projections, and Parameter Wrappers

@propertyWrapper
struct Validated<T> {
    private var value: T
    private var validator: (T) -> Bool

    init(wrappedValue: T, _ validator: @escaping (T) -> Bool) {
        self.value = wrappedValue
        self.validator = validator
    }

    var wrappedValue: T {
        get { value }
        set { if validator(newValue) { value = newValue } }
    }

    var projectedValue: Bool {
        return validator(value)
    }
}

@propertyWrapper
struct Uppercased {
    private var text: String = ""

    init(wrappedValue: String) {
        self.wrappedValue = wrappedValue
    }

    var wrappedValue: String {
        get { text }
        set { text = newValue.uppercased() }
    }

    var projectedValue: Int {
        return text.count
    }
}

struct Account {
    @Uppercased
    var username: String = "admin"

    @Validated({ $0 >= 0 && $0 <= 150 })
    var age: Int = 25

    func inspect() {
        print("Username: \(username), length: \($username)")
        print("Age: \(age), isValid: \($age)")
    }
}

func applyDiscount(@Validated({ $0 > 0.0 && $0 < 1.0 }) rate: Double, to amount: Double) -> Double {
    return amount * (1.0 - rate)
}
