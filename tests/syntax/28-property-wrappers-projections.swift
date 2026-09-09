// §7 / §6.2 Property Wrappers and Projections

@propertyWrapper
struct UserDefault<T> {
    let key: String
    let defaultValue: T

    var wrappedValue: T {
        get { defaultValue }
        set { }
    }

    var projectedValue: UserDefault<T> {
        return self
    }
}

@propertyWrapper
struct Clamped<Value: Comparable> {
    var value: Value
    let range: ClosedRange<Value>

    init(wrappedValue: Value, _ range: ClosedRange<Value>) {
        self.range = range
        self.value = min(max(wrappedValue, range.lowerBound), range.upperBound)
    }

    var wrappedValue: Value {
        get { value }
        set { value = min(max(newValue, range.lowerBound), range.upperBound) }
    }

    var projectedValue: Bool {
        return value == range.upperBound
    }
}

struct AppSettings {
    @UserDefault(key: "is_logged_in", defaultValue: false)
    var isLoggedIn: Bool

    @Clamped(0...100)
    var volume: Int = 75

    func inspect() {
        print(volume)
        print($volume)
        print($isLoggedIn.key)
    }
}
