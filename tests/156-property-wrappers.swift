// A property wrapper supplies a property's storage and its projected value.
@propertyWrapper
struct Clamped {
    private var value: Int
    let range: ClosedRange<Int>
    var wrappedValue: Int {
        get { value }
        set { value = min(max(newValue, range.lowerBound), range.upperBound) }
    }
    var projectedValue: String { "\(value) in \(range)" }
    init(wrappedValue: Int, _ range: ClosedRange<Int>) {
        self.range = range
        value = min(max(wrappedValue, range.lowerBound), range.upperBound)
    }
}
struct Volume {
    @Clamped(0...10) var level = 5
}
var v = Volume()
v.level = 42
print(v.level, v.$level)
v.level = -3
print(v.level)
