// §7 Attributes

// Attribute: @ AttributeName
@objc class LegacyBridgedClass: NSObject {}

// Attribute with AttributeArgumentClause (BalancedTokens)
@available(iOS 13, macOS 10.15, *)
struct ModernFeature {}

@discardableResult
func computeAndMaybeIgnore() -> Int { 42 }

// Multiple attributes (AttributeList: Attribute {Attribute})
@objc @available(iOS 14, *)
class MultiAttributed: NSObject {}

// Attribute on a function parameter/type position
func process(completion: @escaping () -> Void) {
    completion()
}

// Property wrapper attribute (already used earlier, shown again in context)
@propertyWrapper
struct Clamped {
    private var value: Int
    private let range: ClosedRange<Int>
    init(wrappedValue: Int, _ range: ClosedRange<Int>) {
        self.range = range
        self.value = min(max(wrappedValue, range.lowerBound), range.upperBound)
    }
    var wrappedValue: Int {
        get { value }
        set { value = min(max(newValue, range.lowerBound), range.upperBound) }
    }
}

struct Settings {
    @Clamped(0...100) var volume: Int = 50
}

import Foundation