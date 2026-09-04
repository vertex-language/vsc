// §6.2 Constants and Variables

// ConstantDeclaration with PatternInitializerList
let a = 1, b = 2, c = 3

// VariableDeclaration: simple pattern with initializer
var counter = 0

// stored property with type annotation only
var name: String

// Computed property: var VariableName [TypeAnnotation] CodeBlock (getter shorthand)
var doubled: Int { counter * 2 }

// GetterSetterBlock
var total: Int {
    get { counter }
    set { counter = newValue }
}
var totalNamedSetter: Int {
    get { counter }
    set(newTotal) { counter = newTotal }
}

// GetterKeywordClause (protocol-style requirement shown in context of a class)
class Box {
    var value: Int {
        get { 0 }
    }
}

// WillSetDidSetBlock
var observed: Int = 0 {
    willSet { print("about to become \(newValue)") }
    didSet { print("was \(oldValue)") }
}

// DeclarationModifiers on variable/constant declarations
private let hiddenValue = 42
public var exposedValue = 0
struct Scope {
    static var typeLevelValue = 0
}
lazy var lazyValue: Int = { 100 }()