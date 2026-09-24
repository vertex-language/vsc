// §5.3 Conditions

// AvailabilityCondition
func availabilityExample() {
    if #available(iOS 15, macOS 12, *) {
        print("modern APIs available")
    } else {
        print("fallback")
    }

    if #unavailable(iOS 13) {
        print("using legacy path")
    }
}

// CaseCondition: case Pattern Initializer
enum Result<T> { case success(T), failure(Error) }

func caseConditionExample(_ result: Result<Int>) {
    if case .success(let value) = result {
        print(value)
    }
}

// OptionalBindingCondition: let / var Pattern [Initializer]
func optionalBindingExample(_ maybe: Int?) {
    if let value = maybe {
        print(value)
    }
    if var mutableValue = maybe {
        mutableValue += 1
        print(mutableValue)
    }
}

// Multiple Condition in a ConditionList
func multiConditionExample(_ a: Int?, _ b: Int?) {
    if let a, let b, a < b {
        print("\(a) < \(b)")
    }
}