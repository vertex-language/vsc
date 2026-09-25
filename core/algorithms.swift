// The algorithms of the core, written as source: what Swift's standard
// library writes in Swift over a small runtime, this compiler writes in
// the language over its own. Nothing here is compiled ahead of time --
// each is lowered where a program uses it, for the element types it is
// used with, as a generic function of the program's own would be.
//
// core.swift declares what the runtime provides; this file defines what
// is made of that.

// ---- Array ----

extension Array {
    // A new array of what transform makes of each element, in order.
    func map<T>(_ transform: (Element) throws -> T) rethrows -> [T] {
        var out: [T] = []
        for x in self { out.append(try transform(x)) }
        return out
    }

    // The elements isIncluded keeps, in order.
    func filter(_ isIncluded: (Element) throws -> Bool) rethrows -> [Element] {
        var out: [Element] = []
        for x in self where try isIncluded(x) { out.append(x) }
        return out
    }

    // The elements combined in order, starting from initialResult.
    func reduce<Result>(_ initialResult: Result, _ nextPartialResult: (Result, Element) throws -> Result) rethrows -> Result {
        var acc = initialResult
        for x in self { acc = try nextPartialResult(acc, x) }
        return acc
    }

    // What transform makes of each element, with nothing where it makes nil.
    func compactMap<T>(_ transform: (Element) throws -> T?) rethrows -> [T] {
        var out: [T] = []
        for x in self {
            if let y = try transform(x) { out.append(y) }
        }
        return out
    }

    // The arrays transform makes of each element, one after another.
    func flatMap<T>(_ transform: (Element) throws -> [T]) rethrows -> [T] {
        var out: [T] = []
        for x in self { out.append(contentsOf: try transform(x)) }
        return out
    }

    // body, called with each element in order.
    func forEach(_ body: (Element) throws -> Void) rethrows {
        for x in self { try body(x) }
    }

    // Whether any element satisfies predicate.
    func contains(where predicate: (Element) -> Bool) -> Bool {
        for x in self where predicate(x) { return true }
        return false
    }

    // Whether every element satisfies predicate.
    func allSatisfy(_ predicate: (Element) -> Bool) -> Bool {
        for x in self where !predicate(x) { return false }
        return true
    }

    // The first element satisfying predicate, if any.
    func first(where predicate: (Element) -> Bool) -> Element? {
        for x in self where predicate(x) { return x }
        return nil
    }

    // The last element satisfying predicate, if any.
    func last(where predicate: (Element) -> Bool) -> Element? {
        var i = count - 1
        while i >= 0 {
            if predicate(self[i]) { return self[i] }
            i -= 1
        }
        return nil
    }

    // Where the first element satisfying predicate is, if any.
    func firstIndex(where predicate: (Element) -> Bool) -> Int? {
        var i = 0
        while i < count {
            if predicate(self[i]) { return i }
            i += 1
        }
        return nil
    }

    // Where the last element satisfying predicate is, if any.
    func lastIndex(where predicate: (Element) -> Bool) -> Int? {
        var i = count - 1
        while i >= 0 {
            if predicate(self[i]) { return i }
            i -= 1
        }
        return nil
    }

    // Each element paired with its offset from the start.
    func enumerated() -> [(offset: Int, element: Element)] {
        var out: [(offset: Int, element: Element)] = []
        var i = 0
        for x in self {
            out.append((offset: i, element: x))
            i += 1
        }
        return out
    }

    // The elements last to first.
    func reversed() -> [Element] {
        var out: [Element] = []
        var i = count - 1
        while i >= 0 {
            out.append(self[i])
            i -= 1
        }
        return out
    }

    // The elements in the order areInIncreasingOrder puts them: an
    // insertion sort, stable as Swift's sort is.
    func sorted(by areInIncreasingOrder: (Element, Element) -> Bool) -> [Element] {
        var out = self
        var i = 1
        while i < out.count {
            let key = out[i]
            var j = i - 1
            while j >= 0 && areInIncreasingOrder(key, out[j]) {
                out[j + 1] = out[j]
                j -= 1
            }
            out[j + 1] = key
            i += 1
        }
        return out
    }

    // Sorts in place; see sorted(by:).
    mutating func sort(by areInIncreasingOrder: (Element, Element) -> Bool) {
        self = sorted(by: areInIncreasingOrder)
    }

    // The element areInIncreasingOrder puts first, if any.
    func min(by areInIncreasingOrder: (Element, Element) -> Bool) -> Element? {
        if isEmpty { return nil }
        var best = self[0]
        for x in self where areInIncreasingOrder(x, best) { best = x }
        return best
    }

    // The element areInIncreasingOrder puts last, if any.
    func max(by areInIncreasingOrder: (Element, Element) -> Bool) -> Element? {
        if isEmpty { return nil }
        var best = self[0]
        for x in self where areInIncreasingOrder(best, x) { best = x }
        return best
    }

    // The positions of the elements, 0 up to count.
    var indices: Range<Int> { return 0..<count }

    // The elements combined in order into initialResult, which
    // updateAccumulatingResult changes in place.
    func reduce<Result>(into initialResult: Result, _ updateAccumulatingResult: (inout Result, Element) -> Void) -> Result {
        var acc = initialResult
        for x in self { updateAccumulatingResult(&acc, x) }
        return acc
    }

    // Takes the first element away and answers it.
    mutating func removeFirst() -> Element {
        if isEmpty { fatalError("Can't remove first element from an empty collection") }
        return remove(at: 0)
    }

    // Takes the first k elements away.
    mutating func removeFirst(_ k: Int) {
        if k < 0 || k > count { fatalError("Can't remove more items from a collection than it contains") }
        self = Array(dropFirst(k))
    }

    // Takes the last k elements away.
    mutating func removeLast(_ k: Int) {
        if k < 0 || k > count { fatalError("Can't remove more items from a collection than it contains") }
        self = Array(dropLast(k))
    }

    // Takes away every element shouldBeRemoved is true of.
    mutating func removeAll(where shouldBeRemoved: (Element) -> Bool) {
        self = filter { (x: Element) -> Bool in !shouldBeRemoved(x) }
    }

    // The elements in subrange replaced by newElements.
    mutating func replaceSubrange(_ subrange: Range<Int>, with newElements: [Element]) {
        if subrange.lowerBound < 0 || subrange.upperBound > count { fatalError("Array replace: subrange extends past the end") }
        var out: [Element] = []
        var i = 0
        while i < subrange.lowerBound {
            out.append(self[i])
            i += 1
        }
        out.append(contentsOf: newElements)
        i = subrange.upperBound
        while i < count {
            out.append(self[i])
            i += 1
        }
        self = out
    }

    // Room for n elements without growing: a hint, which the runtime's
    // growth does not need.
    mutating func reserveCapacity(_ n: Int) {}

    // The elements of any sequence of them appended, in order: a slice,
    // a range, a string's bytes. An array is appended by the runtime.
    mutating func append<S: Sequence>(contentsOf newElements: S) where S.Element == Element {
        for x in newElements { append(x) }
    }

    // newElements put in before position i.
    mutating func insert(contentsOf newElements: [Element], at i: Int) {
        replaceSubrange(i..<i, with: newElements)
    }

    // The elements at i and j change places.
    mutating func swapAt(_ i: Int, _ j: Int) {
        if i == j { return }
        let t = self[i]
        self[i] = self[j]
        self[j] = t
    }

    // All but the first k elements.
    func dropFirst(_ k: Int = 1) -> [Element] {
        var out: [Element] = []
        var i = k < 0 ? 0 : k
        while i < count {
            out.append(self[i])
            i += 1
        }
        return out
    }

    // All but the last k elements.
    func dropLast(_ k: Int = 1) -> [Element] {
        var out: [Element] = []
        let end = count - (k < 0 ? 0 : k)
        var i = 0
        while i < end {
            out.append(self[i])
            i += 1
        }
        return out
    }

    // The first maxLength elements, or all of them where there are fewer.
    func prefix(_ maxLength: Int) -> [Element] {
        var out: [Element] = []
        var i = 0
        while i < count && i < maxLength {
            out.append(self[i])
            i += 1
        }
        return out
    }

    // The last maxLength elements, or all of them where there are fewer.
    func suffix(_ maxLength: Int) -> [Element] {
        var out: [Element] = []
        var i = count - maxLength
        if i < 0 { i = 0 }
        while i < count {
            out.append(self[i])
            i += 1
        }
        return out
    }

    // The elements up to the first one that fails predicate.
    func prefix(while predicate: (Element) -> Bool) -> [Element] {
        var out: [Element] = []
        for x in self {
            if !predicate(x) { break }
            out.append(x)
        }
        return out
    }

    // How many elements predicate is true of.
    func count(where predicate: (Element) -> Bool) -> Int {
        var n = 0
        for x in self where predicate(x) { n += 1 }
        return n
    }

    // Whether the elements are other's, in order, by areEquivalent.
    func elementsEqual(_ other: [Element], by areEquivalent: (Element, Element) -> Bool) -> Bool {
        if count != other.count { return false }
        var i = 0
        while i < count {
            if !areEquivalent(self[i], other[i]) { return false }
            i += 1
        }
        return true
    }

    // The elements from the first one that fails predicate.
    func drop(while predicate: (Element) -> Bool) -> [Element] {
        var out: [Element] = []
        var dropping = true
        for x in self {
            if dropping && predicate(x) { continue }
            dropping = false
            out.append(x)
        }
        return out
    }
}

// ---- Dictionary ----

extension Dictionary {
    // The keys, and the values, in the dictionary's order.
    var keys: [Key] {
        var out: [Key] = []
        for (k, _) in self { out.append(k) }
        return out
    }

    var values: [Value] {
        var out: [Value] = []
        for (_, v) in self { out.append(v) }
        return out
    }

    // value under key, answering what was there before, if anything.
    mutating func updateValue(_ value: Value, forKey key: Key) -> Value? {
        let old = self[key]
        self[key] = value
        return old
    }

    // The key-value pairs in the order areInIncreasingOrder puts them.
    func sorted(by areInIncreasingOrder: ((key: Key, value: Value), (key: Key, value: Value)) -> Bool) -> [(key: Key, value: Value)] {
        var pairs: [(key: Key, value: Value)] = []
        for (k, v) in self { pairs.append((key: k, value: v)) }
        return pairs.sorted(by: areInIncreasingOrder)
    }

    // The pairs isIncluded keeps, as a dictionary.
    func filter(_ isIncluded: ((key: Key, value: Value)) -> Bool) -> [Key: Value] {
        var out: [Key: Value] = [:]
        for (k, v) in self where isIncluded((key: k, value: v)) { out[k] = v }
        return out
    }

    // The same keys, each with what transform makes of its value.
    func mapValues<T>(_ transform: (Value) -> T) -> [Key: T] {
        var out: [Key: T] = [:]
        for (k, v) in self { out[k] = transform(v) }
        return out
    }

    // What transform makes of each pair, in the dictionary's order.
    func map<T>(_ transform: ((key: Key, value: Value)) -> T) -> [T] {
        var out: [T] = []
        for (k, v) in self { out.append(transform((key: k, value: v))) }
        return out
    }

    // Whether any pair satisfies predicate.
    func contains(where predicate: ((key: Key, value: Value)) -> Bool) -> Bool {
        for (k, v) in self where predicate((key: k, value: v)) { return true }
        return false
    }

    // body, called with each pair.
    func forEach(_ body: ((key: Key, value: Value)) -> Void) {
        for (k, v) in self { body((key: k, value: v)) }
    }

    // The pairs combined in order, starting from initialResult.
    func reduce<Result>(_ initialResult: Result, _ nextPartialResult: (Result, (key: Key, value: Value)) -> Result) -> Result {
        var acc = initialResult
        for (k, v) in self { acc = nextPartialResult(acc, (key: k, value: v)) }
        return acc
    }

    // The pairs of other put in, combine choosing a value where both have
    // the key.
    mutating func merge(_ other: [Key: Value], uniquingKeysWith combine: (Value, Value) -> Value) {
        for (k, v) in other {
            if let mine = self[k] {
                self[k] = combine(mine, v)
            } else {
                self[k] = v
            }
        }
    }
}

// ---- Set ----

extension Set {
    // newMember put in, where it is not in already; whether it was, and
    // the member that is in now.
    mutating func insert(_ newMember: Element) -> (inserted: Bool, memberAfterInsert: Element) {
        if contains(newMember) { return (inserted: false, memberAfterInsert: newMember) }
        _insert(newMember)
        return (inserted: true, memberAfterInsert: newMember)
    }

    // The members of either set.
    func union(_ other: Set<Element>) -> Set<Element> {
        var out = self
        for x in other { out._insert(x) }
        return out
    }

    // The members of both.
    func intersection(_ other: Set<Element>) -> Set<Element> {
        var out = Set<Element>()
        for x in self where other.contains(x) { out._insert(x) }
        return out
    }

    // The members of this set that are not other's.
    func subtracting(_ other: Set<Element>) -> Set<Element> {
        var out = Set<Element>()
        for x in self where !other.contains(x) { out._insert(x) }
        return out
    }

    // The members of one set or the other but not both.
    func symmetricDifference(_ other: Set<Element>) -> Set<Element> {
        var out = subtracting(other)
        for x in other where !contains(x) { out._insert(x) }
        return out
    }

    mutating func formUnion(_ other: Set<Element>) { self = union(other) }
    mutating func formIntersection(_ other: Set<Element>) { self = intersection(other) }
    mutating func subtract(_ other: Set<Element>) { self = subtracting(other) }

    func isSubset(of other: Set<Element>) -> Bool {
        for x in self where !other.contains(x) { return false }
        return true
    }

    func isSuperset(of other: Set<Element>) -> Bool { return other.isSubset(of: self) }

    func isDisjoint(with other: Set<Element>) -> Bool {
        for x in self where other.contains(x) { return false }
        return true
    }

    // A new array of what transform makes of each member.
    func map<T>(_ transform: (Element) -> T) -> [T] {
        var out: [T] = []
        for x in self { out.append(transform(x)) }
        return out
    }

    // The members isIncluded keeps, as a set.
    func filter(_ isIncluded: (Element) -> Bool) -> Set<Element> {
        var out = Set<Element>()
        for x in self where isIncluded(x) { out._insert(x) }
        return out
    }

    // The members combined, starting from initialResult.
    func reduce<Result>(_ initialResult: Result, _ nextPartialResult: (Result, Element) -> Result) -> Result {
        var acc = initialResult
        for x in self { acc = nextPartialResult(acc, x) }
        return acc
    }

    // Whether any member satisfies predicate.
    func contains(where predicate: (Element) -> Bool) -> Bool {
        for x in self where predicate(x) { return true }
        return false
    }

    // The members in the order areInIncreasingOrder puts them.
    func sorted(by areInIncreasingOrder: (Element, Element) -> Bool) -> [Element] {
        var out: [Element] = []
        for x in self { out.append(x) }
        return out.sorted(by: areInIncreasingOrder)
    }
}

extension Set where Element: Comparable {
    // The members in increasing order.
    func sorted() -> [Element] {
        var out: [Element] = []
        for x in self { out.append(x) }
        return out.sorted()
    }

    func min() -> Element? { return sorted().first }
    func max() -> Element? { return sorted().last }
}

// Set(xs) of an array, and of a String's Characters.
func _setOfArray<Element: Hashable>(_ xs: [Element]) -> Set<Element> {
    var out = Set<Element>()
    for x in xs { out._insert(x) }
    return out
}

func _setOfString(_ s: String) -> Set<Character> {
    var out = Set<Character>()
    for c in s { out._insert(c) }
    return out
}

// Dictionary(grouping:by:), Dictionary(_:uniquingKeysWith:) and
// Dictionary(uniqueKeysWithValues:): the checker reads a call of one of
// those as a call of these.

func _dictionaryGrouping<Key: Hashable, Element>(_ values: [Element], _ keyForValue: (Element) -> Key) -> [Key: [Element]] {
    var out: [Key: [Element]] = [:]
    for v in values {
        let k = keyForValue(v)
        if var group = out[k] {
            group.append(v)
            out[k] = group
        } else {
            out[k] = [v]
        }
    }
    return out
}

func _dictionaryUniquing<Key: Hashable, Value>(_ pairs: [(Key, Value)], _ combine: (Value, Value) -> Value) -> [Key: Value] {
    var out: [Key: Value] = [:]
    for (k, v) in pairs {
        if let old = out[k] {
            out[k] = combine(old, v)
        } else {
            out[k] = v
        }
    }
    return out
}

func _dictionaryUniqueKeys<Key: Hashable, Value>(_ pairs: [(Key, Value)]) -> [Key: Value] {
    var out: [Key: Value] = [:]
    for (k, v) in pairs {
        if out[k] != nil { fatalError("Dictionary literal contains duplicate keys") }
        out[k] = v
    }
    return out
}

// ---- ArraySlice ----

extension _ArraySliceIterator: IteratorProtocol {
    mutating func next() -> Element? {
        if _at >= _end { return nil }
        let x = _base[_at]
        _at += 1
        return x
    }
}

extension ArraySlice {
    // The element at i, which counts from the start of the base array, as
    // an ArraySlice's indices do.
    subscript(i: Int) -> Element {
        if i < startIndex || i >= endIndex { fatalError("Index out of range") }
        return base[i]
    }

    func makeIterator() -> _ArraySliceIterator<Element> {
        return _ArraySliceIterator(_base: base, _at: startIndex, _end: endIndex)
    }

    var indices: Range<Int> { return startIndex..<endIndex }
    var first: Element? { return startIndex < endIndex ? base[startIndex] : nil }
    var last: Element? { return startIndex < endIndex ? base[endIndex - 1] : nil }

    // A new array of what transform makes of each element, in order.
    func map<T>(_ transform: (Element) -> T) -> [T] {
        var out: [T] = []
        for x in self { out.append(transform(x)) }
        return out
    }

    // The elements isIncluded keeps, in order.
    func filter(_ isIncluded: (Element) -> Bool) -> [Element] {
        var out: [Element] = []
        for x in self where isIncluded(x) { out.append(x) }
        return out
    }

    // The elements combined in order, starting from initialResult.
    func reduce<Result>(_ initialResult: Result, _ nextPartialResult: (Result, Element) -> Result) -> Result {
        var acc = initialResult
        for x in self { acc = nextPartialResult(acc, x) }
        return acc
    }
}

extension ArraySlice: CustomStringConvertible, CustomDebugStringConvertible {
    var description: String { return "\(Array(self))" }
    var debugDescription: String { return "ArraySlice(\(Array(self)))" }
}

extension Array where Element: Equatable {
    // Whether the elements are other's, in order.
    func elementsEqual(_ other: [Element]) -> Bool {
        if count != other.count { return false }
        var i = 0
        while i < count {
            if self[i] != other[i] { return false }
            i += 1
        }
        return true
    }

    // Whether the first elements are possiblePrefix's.
    func starts(with possiblePrefix: [Element]) -> Bool {
        if possiblePrefix.count > count { return false }
        var i = 0
        while i < possiblePrefix.count {
            if self[i] != possiblePrefix[i] { return false }
            i += 1
        }
        return true
    }

    // The runs of elements between those equal to separator, the empty
    // ones left out unless omittingEmptySubsequences is false.
    func split(separator: Element, maxSplits: Int = Int.max, omittingEmptySubsequences: Bool = true) -> [ArraySlice<Element>] {
        var out: [ArraySlice<Element>] = []
        var start = 0
        var i = 0
        while i < count {
            if self[i] == separator && out.count < maxSplits {
                if i > start || !omittingEmptySubsequences {
                    out.append(ArraySlice(base: self, startIndex: start, endIndex: i))
                }
                start = i + 1
            }
            i += 1
        }
        if count > start || !omittingEmptySubsequences {
            out.append(ArraySlice(base: self, startIndex: start, endIndex: count))
        }
        return out
    }

    // Whether an element equals element. One the runtime hashes is
    // answered by the runtime; this is the rest, compared with ==.
    func contains(_ element: Element) -> Bool {
        return firstIndex(of: element) != nil
    }

    // Where the first element equal to element is, if any.
    func firstIndex(of element: Element) -> Int? {
        var i = 0
        while i < count {
            if self[i] == element { return i }
            i += 1
        }
        return nil
    }

    // Where the last element equal to element is, if any.
    func lastIndex(of element: Element) -> Int? {
        var i = count - 1
        while i >= 0 {
            if self[i] == element { return i }
            i -= 1
        }
        return nil
    }
}

extension Array where Element: Comparable {
    // The elements in increasing order.
    func sorted() -> [Element] {
        return sorted(by: { (a: Element, b: Element) -> Bool in a < b })
    }

    // Sorts in place, into increasing order.
    mutating func sort() {
        self = sorted()
    }

    // Whether the elements come before other's in dictionary order.
    func lexicographicallyPrecedes(_ other: [Element]) -> Bool {
        var i = 0
        while i < count && i < other.count {
            if self[i] < other[i] { return true }
            if other[i] < self[i] { return false }
            i += 1
        }
        return count < other.count
    }

    // The least element, if any.
    func min() -> Element? {
        if isEmpty { return nil }
        var best = self[0]
        for x in self where x < best { best = x }
        return best
    }

    // The greatest element, if any.
    func max() -> Element? {
        if isEmpty { return nil }
        var best = self[0]
        for x in self where best < x { best = x }
        return best
    }
}

// What an array is to an extension of arrays of arrays: its elements,
// by their type.
protocol _ArrayProtocol {
    associatedtype Element
    var _elements: [Element] { get }
}

extension Array: _ArrayProtocol {
    var _elements: [Element] { return self }
}

extension Array where Element: _ArrayProtocol {
    // The elements of the arrays, one array after another.
    func joined() -> [Element.Element] {
        var out: [Element.Element] = []
        for xs in self { out.append(contentsOf: xs._elements) }
        return out
    }
}

extension Array where Element == String {
    // The strings one after another, separator between each pair.
    func joined(separator: String = "") -> String {
        var out = ""
        var first = true
        for s in self {
            if !first { out += separator }
            out += s
            first = false
        }
        return out
    }
}

extension Int {
    // The number text writes in radix, or nil where it writes none: an
    // optional sign, then digits and letters, none past the radix.
    init?(_ text: String, radix: Int) {
        if radix < 2 || radix > 36 { fatalError("Radix must be between 2 and 36") }
        var result = 0
        var negative = false
        var digits = 0
        var first = true
        for c in text {
            if first && (c == "-" || c == "+") {
                negative = c == "-"
                first = false
                continue
            }
            first = false
            guard let code = c.asciiValue else { return nil }
            var d = radix
            if code >= 48 && code <= 57 {
                d = Int(code) - 48
            } else if code >= 97 && code <= 122 {
                d = Int(code) - 97 + 10
            } else if code >= 65 && code <= 90 {
                d = Int(code) - 65 + 10
            }
            if d >= radix { return nil }
            let (shifted, over) = result.multipliedReportingOverflow(by: radix)
            let (next, over2) = negative ? shifted.subtractingReportingOverflow(d) : shifted.addingReportingOverflow(d)
            if over || over2 { return nil }
            result = next
            digits += 1
        }
        if digits == 0 { return nil }
        self = result
    }
}

// What print and interpolation show of a number or a Bool, as a String.
extension Int {
    var description: String { return String(self) }
}

extension Int8 {
    var description: String { return String(self) }
}

extension Int16 {
    var description: String { return String(self) }
}

extension Int32 {
    var description: String { return String(self) }
}

extension Int64 {
    var description: String { return String(self) }
}

extension UInt {
    var description: String { return String(self) }
}

extension UInt8 {
    var description: String { return String(self) }
}

extension UInt16 {
    var description: String { return String(self) }
}

extension UInt32 {
    var description: String { return String(self) }
}

extension UInt64 {
    var description: String { return String(self) }
}

extension Double {
    var description: String { return String(self) }
}

extension Float {
    var description: String { return String(self) }
}

extension Bool {
    var description: String { return String(self) }
}

// ---- Optional ----

extension Optional {
    // What transform makes of the value, or nil where there is none.
    func map<U>(_ transform: (Wrapped) -> U) -> U? {
        if let w = self { return transform(w) }
        return nil
    }

    // What transform makes of the value, which may itself be nil.
    func flatMap<U>(_ transform: (Wrapped) -> U?) -> U? {
        if let w = self { return transform(w) }
        return nil
    }
}

// ---- Result ----

extension Result {
    // The success value, or the failure thrown.
    func get() throws -> Success {
        switch self {
        case .success(let value): return value
        case .failure(let error): throw error
        }
    }

    // A success transformed; a failure as it is.
    func map<NewSuccess>(_ transform: (Success) -> NewSuccess) -> Result<NewSuccess, Failure> {
        switch self {
        case .success(let value): return .success(transform(value))
        case .failure(let error): return .failure(error)
        }
    }

    // A failure transformed; a success as it is.
    func mapError<NewFailure>(_ transform: (Failure) -> NewFailure) -> Result<Success, NewFailure> {
        switch self {
        case .success(let value): return .success(value)
        case .failure(let error): return .failure(transform(error))
        }
    }

    // A success transformed into another result; a failure as it is.
    func flatMap<NewSuccess>(_ transform: (Success) -> Result<NewSuccess, Failure>) -> Result<NewSuccess, Failure> {
        switch self {
        case .success(let value): return transform(value)
        case .failure(let error): return .failure(error)
        }
    }
}

extension Result where Failure == any Error {
    // What body returns, or the error it throws.
    init(catching body: () throws -> Success) {
        do {
            self = .success(try body())
        } catch {
            self = .failure(error)
        }
    }
}

// ---- Character ----

extension Character: ExpressibleByExtendedGraphemeClusterLiteral {
    init(extendedGraphemeClusterLiteral value: String) {
        _string = value
    }
}

extension Character: Equatable, Hashable, Comparable {
    // Characters are equal when their clusters are canonically
    // equivalent, as Strings are.
    static func == (lhs: Character, rhs: Character) -> Bool {
        return lhs._string == rhs._string
    }

    // Ordered as the Strings of them are.
    static func < (lhs: Character, rhs: Character) -> Bool {
        return lhs._string < rhs._string
    }

    func hash(into hasher: inout Hasher) {
        hasher.combine(_string)
    }
}

extension Character: CustomStringConvertible, CustomDebugStringConvertible {
    var description: String { return _string }
    var debugDescription: String { return _string.debugDescription }
}

// What a Character's properties ask of it, as Swift's do: most of its
// first scalar, the scalar's properties the runtime's tables hold.
extension Character {
    init(_ scalar: UnicodeScalar) { _string = _scalarString(scalar.value) }

    var unicodeScalars: _UnicodeScalarView { return _UnicodeScalarView(_string: _string) }

    var _firstScalar: UInt32 { return UInt32(_scalarAt(_string, 0) & 0x1FFFFF) }
    var _isSingleScalar: Bool { return _scalarAt(_string, 0) >> 21 == _utf8Count(_string) }
    var _properties: UInt32 { return _scalarProperties(_firstScalar) }
    var _generalCategory: UInt32 { return (_properties >> 8) & 0xFF }

    // The ASCII code of the Character, with CR LF a newline's.
    var asciiValue: UInt8? {
        if _string == "\r\n" { return 10 }
        if !_isSingleScalar || _firstScalar >= 0x80 { return nil }
        return UInt8(_firstScalar)
    }
    var isASCII: Bool { return asciiValue != nil }

    var isLetter: Bool { return _properties & 1 != 0 }
    var isWhitespace: Bool { return _properties & 16 != 0 }
    var isMathSymbol: Bool { return _properties & 32 != 0 }
    var isNumber: Bool { return (_properties >> 16) & 0xFF != 0 }
    var isPunctuation: Bool { return _generalCategory >= 12 && _generalCategory <= 18 }
    var isSymbol: Bool { return _generalCategory >= 19 && _generalCategory <= 22 }
    var isCurrencySymbol: Bool { return _generalCategory == 20 }
    var isNewline: Bool {
        let c = _firstScalar
        return (c >= 0x0A && c <= 0x0D) || c == 0x85 || c == 0x2028 || c == 0x2029
    }

    var isCased: Bool {
        if _isSingleScalar && _properties & 8 != 0 { return true }
        return _string != _uppercased(_string) || _string != _lowercased(_string)
    }
    var isUppercase: Bool {
        if _isSingleScalar && _properties & 2 != 0 { return true }
        return _string == _uppercased(_string) && isCased
    }
    var isLowercase: Bool {
        if _isSingleScalar && _properties & 4 != 0 { return true }
        return _string == _lowercased(_string) && isCased
    }

    // The whole number the Character stands for, if it is one scalar that
    // has one: 7 for "7", 12 for "Ⅻ".
    var wholeNumberValue: Int? {
        if !_isSingleScalar { return nil }
        let n = _scalarWholeNumber(_firstScalar)
        return n < 0 ? nil : n
    }
    var isWholeNumber: Bool { return wholeNumberValue != nil }

    var hexDigitValue: Int? {
        if !_isSingleScalar { return nil }
        let c = Int(_firstScalar)
        if c >= 0x30 && c <= 0x39 { return c - 0x30 }
        if c >= 0x41 && c <= 0x46 { return c - 0x41 + 10 }
        if c >= 0x61 && c <= 0x66 { return c - 0x61 + 10 }
        if c >= 0xFF10 && c <= 0xFF19 { return c - 0xFF10 }
        if c >= 0xFF21 && c <= 0xFF26 { return c - 0xFF21 + 10 }
        if c >= 0xFF41 && c <= 0xFF46 { return c - 0xFF41 + 10 }
        return nil
    }
    var isHexDigit: Bool { return hexDigitValue != nil }

    func uppercased() -> String { return _uppercased(_string) }
    func lowercased() -> String { return _lowercased(_string) }
}

// ---- Unicode scalars ----

extension UnicodeScalar {
    init(_ v: UInt8) { value = UInt32(v) }

    init?(_ v: UInt32) {
        if v > 0x10FFFF || (v >= 0xD800 && v <= 0xDFFF) { return nil }
        value = v
    }

    init?(_ v: Int) {
        if v < 0 || v > 0x10FFFF || (v >= 0xD800 && v <= 0xDFFF) { return nil }
        value = UInt32(v)
    }

    var isASCII: Bool { return value < 0x80 }
}

extension UnicodeScalar: ExpressibleByUnicodeScalarLiteral {
    init(unicodeScalarLiteral scalar: String) {
        value = UInt32(_scalarAt(scalar, 0) & 0x1FFFFF)
    }
}

extension UnicodeScalar: Equatable, Hashable, Comparable {
    static func == (lhs: UnicodeScalar, rhs: UnicodeScalar) -> Bool { return lhs.value == rhs.value }
    static func < (lhs: UnicodeScalar, rhs: UnicodeScalar) -> Bool { return lhs.value < rhs.value }
    func hash(into hasher: inout Hasher) { hasher.combine(value) }
}

extension UnicodeScalar: CustomStringConvertible, CustomDebugStringConvertible {
    var description: String { return _scalarString(value) }
    var debugDescription: String { return _scalarString(value).debugDescription }
}

extension _UnicodeScalarIterator: IteratorProtocol {
    // The next scalar, or nil past the last.
    mutating func next() -> UnicodeScalar? {
        let packed = _scalarAt(_string, _at)
        let end = packed >> 21
        if end == _at { return nil }
        _at = end
        return UnicodeScalar(value: UInt32(packed & 0x1FFFFF))
    }
}

extension _UnicodeScalarView {
    // scalar, after the others.
    mutating func append(_ scalar: UnicodeScalar) {
        _string.append(Character(scalar))
    }

    func makeIterator() -> _UnicodeScalarIterator {
        return _UnicodeScalarIterator(_string: _string, _at: 0)
    }

    var count: Int {
        var n = 0
        for _ in self { n += 1 }
        return n
    }

    var isEmpty: Bool { return _string.isEmpty }

    var first: UnicodeScalar? {
        for s in self { return s }
        return nil
    }

    // A new array of what transform makes of each scalar, in order.
    func map<T>(_ transform: (UnicodeScalar) -> T) -> [T] {
        var out: [T] = []
        for s in self { out.append(transform(s)) }
        return out
    }
}

extension _UnicodeScalarView: CustomStringConvertible, CustomDebugStringConvertible {
    var description: String { return _string }
    var debugDescription: String { return _string.debugDescription }
}

extension _UTF16View {
    // How many UTF-16 code units: two for a scalar past the first plane.
    var count: Int {
        var n = 0
        for s in _UnicodeScalarView(_string: _string) { n += s.value > 0xFFFF ? 2 : 1 }
        return n
    }

    var isEmpty: Bool { return _string.isEmpty }
}

extension _UTF16View: CustomStringConvertible {
    var description: String { return _string }
}

// ---- String, as a collection of Characters ----

extension _StringIndex: Equatable, Hashable, Comparable {
    static func == (lhs: _StringIndex, rhs: _StringIndex) -> Bool { return lhs._offset == rhs._offset }
    static func < (lhs: _StringIndex, rhs: _StringIndex) -> Bool { return lhs._offset < rhs._offset }
    func hash(into hasher: inout Hasher) { hasher.combine(_offset) }
}

extension _StringIterator: IteratorProtocol {
    // The next Character, or nil past the last.
    mutating func next() -> Character? {
        if _at >= _end { return nil }
        let end = _characterEnd(_string, _at)
        let c = Character(_string: _stringSlice(_string, _at, end))
        _at = end
        return c
    }
}

extension String {
    func makeIterator() -> _StringIterator {
        return _StringIterator(_string: self, _at: 0, _end: _utf8Count(self))
    }

    var startIndex: _StringIndex { return _StringIndex(_offset: 0) }
    var endIndex: _StringIndex { return _StringIndex(_offset: _utf8Count(self)) }

    func index(after i: _StringIndex) -> _StringIndex {
        if i._offset >= _utf8Count(self) { fatalError("String index is out of bounds") }
        return _StringIndex(_offset: _characterEnd(self, i._offset))
    }

    func index(before i: _StringIndex) -> _StringIndex {
        if i._offset <= 0 { fatalError("String index is out of bounds") }
        return _StringIndex(_offset: _characterStart(self, i._offset))
    }

    func index(_ i: _StringIndex, offsetBy distance: Int) -> _StringIndex {
        var at = i._offset
        var n = distance
        while n > 0 {
            if at >= _utf8Count(self) { fatalError("String index is out of bounds") }
            at = _characterEnd(self, at)
            n -= 1
        }
        while n < 0 {
            if at <= 0 { fatalError("String index is out of bounds") }
            at = _characterStart(self, at)
            n += 1
        }
        return _StringIndex(_offset: at)
    }

    func distance(from start: _StringIndex, to end: _StringIndex) -> Int {
        var n = 0
        var at = start._offset
        while at < end._offset {
            at = _characterEnd(self, at)
            n += 1
        }
        while at > end._offset {
            at = _characterStart(self, at)
            n -= 1
        }
        return n
    }

    subscript(i: _StringIndex) -> Character {
        let end = _characterEnd(self, i._offset)
        if end == i._offset { fatalError("String index is out of bounds") }
        return Character(_string: _stringSlice(self, i._offset, end))
    }

    subscript(r: Range<_StringIndex>) -> Substring {
        if r.upperBound._offset > _utf8Count(self) { fatalError("String index is out of bounds") }
        return Substring(_base: self, _start: r.lowerBound._offset, _end: r.upperBound._offset)
    }

    subscript(r: ClosedRange<_StringIndex>) -> Substring {
        return Substring(_base: self, _start: r.lowerBound._offset, _end: index(after: r.upperBound)._offset)
    }

    // The first and last Characters, if there are any.
    var first: Character? { return isEmpty ? nil : self[startIndex] }
    var last: Character? { return isEmpty ? nil : self[index(before: endIndex)] }

    // The Characters, last first.
    func reversed() -> [Character] {
        var out: [Character] = []
        for c in self { out.append(c) }
        return out.reversed()
    }

    // The first maxLength Characters, or all of them.
    func prefix(_ maxLength: Int) -> Substring {
        var at = 0
        var n = 0
        let count = _utf8Count(self)
        while n < maxLength && at < count {
            at = _characterEnd(self, at)
            n += 1
        }
        return Substring(_base: self, _start: 0, _end: at)
    }

    // The last maxLength Characters, or all of them.
    func suffix(_ maxLength: Int) -> Substring {
        var at = _utf8Count(self)
        var n = 0
        while n < maxLength && at > 0 {
            at = _characterStart(self, at)
            n += 1
        }
        return Substring(_base: self, _start: at, _end: _utf8Count(self))
    }

    // All but the first k Characters.
    func dropFirst(_ k: Int = 1) -> Substring {
        return Substring(_base: self, _start: prefix(k)._end, _end: _utf8Count(self))
    }

    // All but the last k Characters.
    func dropLast(_ k: Int = 1) -> Substring {
        return Substring(_base: self, _start: 0, _end: suffix(k)._start)
    }

    // The Characters, one after another.
    init(_ characters: [Character]) {
        var out = ""
        for c in characters { out += c._string }
        self = out
    }

    // The Characters a Substring holds, as a String of their own.
    init(_ substring: Substring) { self = substring._string }

    // c, after the Characters already there.
    mutating func append(_ c: Character) { self += c._string }

    // A new array of what transform makes of each Character, in order.
    func map<T>(_ transform: (Character) -> T) -> [T] {
        var out: [T] = []
        for c in self { out.append(transform(c)) }
        return out
    }

    // The Characters isIncluded keeps, as a String.
    func filter(_ isIncluded: (Character) -> Bool) -> String {
        var out = ""
        for c in self where isIncluded(c) { out += c._string }
        return out
    }

    // body, called with each Character in order.
    func forEach(_ body: (Character) -> Void) {
        for c in self { body(c) }
    }

    // Whether any Character satisfies predicate.
    func contains(where predicate: (Character) -> Bool) -> Bool {
        for c in self where predicate(c) { return true }
        return false
    }

    // Whether every Character satisfies predicate.
    func allSatisfy(_ predicate: (Character) -> Bool) -> Bool {
        for c in self where !predicate(c) { return false }
        return true
    }

    // The Characters combined in order, starting from initialResult.
    func reduce<Result>(_ initialResult: Result, _ nextPartialResult: (Result, Character) -> Result) -> Result {
        var acc = initialResult
        for c in self { acc = nextPartialResult(acc, c) }
        return acc
    }

    // Where the first Character equal to c is, if any.
    func firstIndex(of c: Character) -> _StringIndex? {
        var at = 0
        let count = _utf8Count(self)
        while at < count {
            let end = _characterEnd(self, at)
            if Character(_string: _stringSlice(self, at, end)) == c { return _StringIndex(_offset: at) }
            at = end
        }
        return nil
    }

    // Whether the String holds c.
    func contains(_ c: Character) -> Bool { return firstIndex(of: c) != nil }

    var unicodeScalars: _UnicodeScalarView {
        get { return _UnicodeScalarView(_string: self) }
        set { self = newValue._string }
    }
    var utf16: _UTF16View { return _UTF16View(_string: self) }

    // The one scalar, as a String.
    init(_ scalar: UnicodeScalar) { self = _scalarString(scalar.value) }

    // value written in radix, its letters in uppercase where asked.
    init(_ value: Int, radix: Int, uppercase: Bool = false) {
        if radix < 2 || radix > 36 { fatalError("Radix must be between 2 and 36") }
        let digits = Array(uppercase ? "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ" : "0123456789abcdefghijklmnopqrstuvwxyz")
        var reversed: [Character] = []
        var n = value
        repeat {
            let r = n % radix
            reversed.append(digits[r < 0 ? -r : r])
            n /= radix
        } while n != 0
        var out = value < 0 ? "-" : ""
        var i = reversed.count - 1
        while i >= 0 {
            out.append(reversed[i])
            i -= 1
        }
        self = out
    }

    // The digits of an unsigned value in radix.
    init(_ value: UInt64, radix: Int, uppercase: Bool = false) {
        if radix < 2 || radix > 36 { fatalError("Radix must be between 2 and 36") }
        let digits = Array(uppercase ? "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ" : "0123456789abcdefghijklmnopqrstuvwxyz")
        let base = UInt64(radix)
        var reversed: [Character] = []
        var n = value
        repeat {
            reversed.append(digits[Int(n % base)])
            n /= base
        } while n != 0
        var out = ""
        var i = reversed.count - 1
        while i >= 0 {
            out.append(reversed[i])
            i -= 1
        }
        self = out
    }

    // The other integer types' digits, as Swift's generic initializer
    // gives them.
    init(_ value: UInt, radix: Int, uppercase: Bool = false) { self = String(UInt64(value), radix: radix, uppercase: uppercase) }
    init(_ value: UInt32, radix: Int, uppercase: Bool = false) { self = String(Int(value), radix: radix, uppercase: uppercase) }
    init(_ value: UInt16, radix: Int, uppercase: Bool = false) { self = String(Int(value), radix: radix, uppercase: uppercase) }
    init(_ value: UInt8, radix: Int, uppercase: Bool = false) { self = String(Int(value), radix: radix, uppercase: uppercase) }
    init(_ value: Int64, radix: Int, uppercase: Bool = false) { self = String(Int(value), radix: radix, uppercase: uppercase) }
    init(_ value: Int32, radix: Int, uppercase: Bool = false) { self = String(Int(value), radix: radix, uppercase: uppercase) }
    init(_ value: Int16, radix: Int, uppercase: Bool = false) { self = String(Int(value), radix: radix, uppercase: uppercase) }
    init(_ value: Int8, radix: Int, uppercase: Bool = false) { self = String(Int(value), radix: radix, uppercase: uppercase) }

    // Every scalar in its uppercase, or lowercase, form.
    func uppercased() -> String { return _uppercased(self) }
    func lowercased() -> String { return _lowercased(self) }

    // The Substrings between the separators, in order: without the empty
    // ones unless asked for them, and at most maxSplits + 1 of them.
    func split(separator: Character, maxSplits: Int = Int.max, omittingEmptySubsequences: Bool = true) -> [Substring] {
        var out: [Substring] = []
        var start = 0
        var at = 0
        let count = _utf8Count(self)
        var splits = 0
        while at < count {
            let end = _characterEnd(self, at)
            if splits < maxSplits && Character(_string: _stringSlice(self, at, end)) == separator {
                if !(omittingEmptySubsequences && start == at) {
                    out.append(Substring(_base: self, _start: start, _end: at))
                    splits += 1
                }
                start = end
            }
            at = end
        }
        if !(omittingEmptySubsequences && start == count) {
            out.append(Substring(_base: self, _start: start, _end: count))
        }
        return out
    }

    // The String with every run of target's Characters replaced by
    // replacement, left to right.
    func replacing(_ target: String, with replacement: String) -> String {
        let chars = Array(self)
        let find = Array(target)
        if find.isEmpty { return self }
        var out = ""
        var i = 0
        while i < chars.count {
            var hit = i + find.count <= chars.count
            var k = 0
            while hit && k < find.count {
                if chars[i + k] != find[k] { hit = false }
                k += 1
            }
            if hit {
                out += replacement
                i += find.count
            } else {
                out += chars[i]._string
                i += 1
            }
        }
        return out
    }

    // Each Character paired with its offset from the start.
    func enumerated() -> [(offset: Int, element: Character)] {
        var out: [(offset: Int, element: Character)] = []
        var i = 0
        for c in self {
            out.append((offset: i, element: c))
            i += 1
        }
        return out
    }

    // The Characters in increasing order.
    func sorted() -> [Character] {
        var out: [Character] = []
        for c in self { out.append(c) }
        return out.sorted()
    }

    // The one Character, as a String.
    init(_ c: Character) { self = c._string }

    // repeatedValue, count times over.
    init(repeating repeatedValue: String, count: Int) {
        var out = ""
        var i = 0
        while i < count {
            out += repeatedValue
            i += 1
        }
        self = out
    }

    // repeatedValue, count times over.
    init(repeating repeatedValue: Character, count: Int) {
        self = String(repeating: repeatedValue._string, count: count)
    }
}

extension Substring {
    // The Characters, as a String of their own.
    var _string: String { return _stringSlice(_base, _start, _end) }

    var startIndex: _StringIndex { return _StringIndex(_offset: _start) }
    var endIndex: _StringIndex { return _StringIndex(_offset: _end) }
    var isEmpty: Bool { return _start == _end }
    var count: Int { return _string.count }

    func makeIterator() -> _StringIterator {
        return _StringIterator(_string: _base, _at: _start, _end: _end)
    }

    subscript(i: _StringIndex) -> Character {
        if i._offset < _start || i._offset >= _end { fatalError("Substring index is out of bounds") }
        return _base[i]
    }

    var first: Character? { return isEmpty ? nil : _base[startIndex] }
    var last: Character? { return isEmpty ? nil : _base[_base.index(before: endIndex)] }

    func hasPrefix(_ prefix: String) -> Bool { return _string.hasPrefix(prefix) }
    func hasSuffix(_ suffix: String) -> Bool { return _string.hasSuffix(suffix) }
}

// A string and the characters of a substring after it.
func + (lhs: String, rhs: Substring) -> String { return lhs + String(rhs) }

// The characters of a substring and a string after them.
func + (lhs: Substring, rhs: String) -> String { return String(lhs) + rhs }

extension Substring: ExpressibleByStringLiteral {
    init(stringLiteral value: String) {
        _base = value
        _start = 0
        _end = _utf8Count(value)
    }
}

extension Substring: Equatable, Hashable, Comparable {
    static func == (lhs: Substring, rhs: Substring) -> Bool { return lhs._string == rhs._string }
    static func < (lhs: Substring, rhs: Substring) -> Bool { return lhs._string < rhs._string }
    func hash(into hasher: inout Hasher) { hasher.combine(_string) }
}

extension Substring: CustomStringConvertible, CustomDebugStringConvertible {
    var description: String { return _string }
    var debugDescription: String { return _string.debugDescription }
}

// ---- Ranges ----

extension Range {
    // Whether element lies between the bounds: at or past lowerBound,
    // short of upperBound.
    func contains(_ element: Bound) -> Bool {
        return lowerBound <= element && element < upperBound
    }

    // Whether the range holds nothing, its bounds equal.
    var isEmpty: Bool { return lowerBound == upperBound }

    // This range, cut to what lies inside limits.
    func clamped(to limits: Range<Bound>) -> Range<Bound> {
        let lower = limits.lowerBound > lowerBound ? limits.lowerBound
            : limits.upperBound < lowerBound ? limits.upperBound : lowerBound
        let upper = limits.upperBound < upperBound ? limits.upperBound
            : limits.lowerBound > upperBound ? limits.lowerBound : upperBound
        return lower..<upper
    }

    // Whether the two ranges share an element.
    func overlaps(_ other: Range<Bound>) -> Bool {
        let neitherEmpty = lowerBound < upperBound && other.lowerBound < other.upperBound
        return neitherEmpty && lowerBound < other.upperBound && other.lowerBound < upperBound
    }

    // Whether the two ranges share an element.
    func overlaps(_ other: ClosedRange<Bound>) -> Bool {
        return lowerBound < upperBound && other.lowerBound < upperBound && lowerBound <= other.upperBound
    }
}

extension Range: CustomStringConvertible {
    var description: String { return "\(lowerBound)..<\(upperBound)" }
}

extension ClosedRange {
    // Whether element lies between the bounds, either included.
    func contains(_ element: Bound) -> Bool {
        return lowerBound <= element && element <= upperBound
    }

    // Always false: a closed range holds at least its bounds.
    var isEmpty: Bool { return false }

    // This range, cut to what lies inside limits.
    func clamped(to limits: ClosedRange<Bound>) -> ClosedRange<Bound> {
        let lower = limits.lowerBound > lowerBound ? limits.lowerBound
            : limits.upperBound < lowerBound ? limits.upperBound : lowerBound
        let upper = limits.upperBound < upperBound ? limits.upperBound
            : limits.lowerBound > upperBound ? limits.lowerBound : upperBound
        return lower...upper
    }

    // Whether the two ranges share an element.
    func overlaps(_ other: ClosedRange<Bound>) -> Bool {
        return lowerBound <= other.upperBound && other.lowerBound <= upperBound
    }

    // Whether the two ranges share an element.
    func overlaps(_ other: Range<Bound>) -> Bool {
        return other.lowerBound < other.upperBound && lowerBound < other.upperBound && other.lowerBound <= upperBound
    }
}

extension ClosedRange: CustomStringConvertible {
    var description: String { return "\(lowerBound)...\(upperBound)" }
}

extension PartialRangeUpTo {
    // Whether element is short of upperBound.
    func contains(_ element: Bound) -> Bool { return element < upperBound }
}

extension PartialRangeThrough {
    // Whether element is at or short of upperBound.
    func contains(_ element: Bound) -> Bool { return element <= upperBound }
}

extension PartialRangeFrom {
    // Whether element is at or past lowerBound.
    func contains(_ element: Bound) -> Bool { return lowerBound <= element }
}

// A range of integers is a collection of them, counted out in order. Swift
// says so of every Strideable bound with an integer stride; these say it
// of Int.

extension _IntRangeIterator: IteratorProtocol {
    mutating func next() -> Int? {
        if _at >= _end { return nil }
        let x = _at
        _at += 1
        return x
    }
}

extension Range where Bound == Int {
    // How many integers the range holds.
    var count: Int { return upperBound - lowerBound }

    func makeIterator() -> _IntRangeIterator {
        return _IntRangeIterator(_at: lowerBound, _end: upperBound)
    }

    // The integers, last first.
    func reversed() -> [Int] {
        var out: [Int] = []
        var i = upperBound
        while i > lowerBound {
            i -= 1
            out.append(i)
        }
        return out
    }

    // A new array of what transform makes of each integer, in order.
    func map<T>(_ transform: (Int) -> T) -> [T] {
        var out: [T] = []
        for x in self { out.append(transform(x)) }
        return out
    }

    // The integers isIncluded keeps, in order.
    func filter(_ isIncluded: (Int) -> Bool) -> [Int] {
        var out: [Int] = []
        for x in self where isIncluded(x) { out.append(x) }
        return out
    }

    // The integers combined in order, starting from initialResult.
    func reduce<Result>(_ initialResult: Result, _ nextPartialResult: (Result, Int) -> Result) -> Result {
        var acc = initialResult
        for x in self { acc = nextPartialResult(acc, x) }
        return acc
    }

    // body, called with each integer in order.
    func forEach(_ body: (Int) -> Void) {
        for x in self { body(x) }
    }
}

extension ClosedRange where Bound == Int {
    // How many integers the range holds.
    var count: Int { return upperBound - lowerBound + 1 }

    func makeIterator() -> _IntRangeIterator {
        return _IntRangeIterator(_at: lowerBound, _end: upperBound + 1)
    }

    // The integers, last first.
    func reversed() -> [Int] {
        var out: [Int] = []
        var i = upperBound
        while i >= lowerBound {
            out.append(i)
            if i == lowerBound { break }
            i -= 1
        }
        return out
    }

    // A new array of what transform makes of each integer, in order.
    func map<T>(_ transform: (Int) -> T) -> [T] {
        var out: [T] = []
        for x in self { out.append(transform(x)) }
        return out
    }

    // The integers isIncluded keeps, in order.
    func filter(_ isIncluded: (Int) -> Bool) -> [Int] {
        var out: [Int] = []
        for x in self where isIncluded(x) { out.append(x) }
        return out
    }

    // The integers combined in order, starting from initialResult.
    func reduce<Result>(_ initialResult: Result, _ nextPartialResult: (Result, Int) -> Result) -> Result {
        var acc = initialResult
        for x in self { acc = nextPartialResult(acc, x) }
        return acc
    }

    // body, called with each integer in order.
    func forEach(_ body: (Int) -> Void) {
        for x in self { body(x) }
    }
}

// ---- Tuples ----

// Tuples of two to six Equatable or Comparable elements compare element
// by element, left to right, as Swift's do.

func == <A: Equatable, B: Equatable>(lhs: (A, B), rhs: (A, B)) -> Bool {
    return lhs.0 == rhs.0 && lhs.1 == rhs.1
}

func != <A: Equatable, B: Equatable>(lhs: (A, B), rhs: (A, B)) -> Bool {
    return !(lhs.0 == rhs.0) || !(lhs.1 == rhs.1)
}

func < <A: Comparable, B: Comparable>(lhs: (A, B), rhs: (A, B)) -> Bool {
    if !(lhs.0 == rhs.0) { return lhs.0 < rhs.0 }
    return lhs.1 < rhs.1
}

func <= <A: Comparable, B: Comparable>(lhs: (A, B), rhs: (A, B)) -> Bool {
    if !(lhs.0 == rhs.0) { return lhs.0 < rhs.0 }
    return !(rhs.1 < lhs.1)
}

func > <A: Comparable, B: Comparable>(lhs: (A, B), rhs: (A, B)) -> Bool {
    if !(lhs.0 == rhs.0) { return rhs.0 < lhs.0 }
    return rhs.1 < lhs.1
}

func >= <A: Comparable, B: Comparable>(lhs: (A, B), rhs: (A, B)) -> Bool {
    if !(lhs.0 == rhs.0) { return rhs.0 < lhs.0 }
    return !(lhs.1 < rhs.1)
}

func == <A: Equatable, B: Equatable, C: Equatable>(lhs: (A, B, C), rhs: (A, B, C)) -> Bool {
    return lhs.0 == rhs.0 && lhs.1 == rhs.1 && lhs.2 == rhs.2
}

func != <A: Equatable, B: Equatable, C: Equatable>(lhs: (A, B, C), rhs: (A, B, C)) -> Bool {
    return !(lhs.0 == rhs.0) || !(lhs.1 == rhs.1) || !(lhs.2 == rhs.2)
}

func < <A: Comparable, B: Comparable, C: Comparable>(lhs: (A, B, C), rhs: (A, B, C)) -> Bool {
    if !(lhs.0 == rhs.0) { return lhs.0 < rhs.0 }
    if !(lhs.1 == rhs.1) { return lhs.1 < rhs.1 }
    return lhs.2 < rhs.2
}

func <= <A: Comparable, B: Comparable, C: Comparable>(lhs: (A, B, C), rhs: (A, B, C)) -> Bool {
    if !(lhs.0 == rhs.0) { return lhs.0 < rhs.0 }
    if !(lhs.1 == rhs.1) { return lhs.1 < rhs.1 }
    return !(rhs.2 < lhs.2)
}

func > <A: Comparable, B: Comparable, C: Comparable>(lhs: (A, B, C), rhs: (A, B, C)) -> Bool {
    if !(lhs.0 == rhs.0) { return rhs.0 < lhs.0 }
    if !(lhs.1 == rhs.1) { return rhs.1 < lhs.1 }
    return rhs.2 < lhs.2
}

func >= <A: Comparable, B: Comparable, C: Comparable>(lhs: (A, B, C), rhs: (A, B, C)) -> Bool {
    if !(lhs.0 == rhs.0) { return rhs.0 < lhs.0 }
    if !(lhs.1 == rhs.1) { return rhs.1 < lhs.1 }
    return !(lhs.2 < rhs.2)
}

func == <A: Equatable, B: Equatable, C: Equatable, D: Equatable>(lhs: (A, B, C, D), rhs: (A, B, C, D)) -> Bool {
    return lhs.0 == rhs.0 && lhs.1 == rhs.1 && lhs.2 == rhs.2 && lhs.3 == rhs.3
}

func != <A: Equatable, B: Equatable, C: Equatable, D: Equatable>(lhs: (A, B, C, D), rhs: (A, B, C, D)) -> Bool {
    return !(lhs.0 == rhs.0) || !(lhs.1 == rhs.1) || !(lhs.2 == rhs.2) || !(lhs.3 == rhs.3)
}

func < <A: Comparable, B: Comparable, C: Comparable, D: Comparable>(lhs: (A, B, C, D), rhs: (A, B, C, D)) -> Bool {
    if !(lhs.0 == rhs.0) { return lhs.0 < rhs.0 }
    if !(lhs.1 == rhs.1) { return lhs.1 < rhs.1 }
    if !(lhs.2 == rhs.2) { return lhs.2 < rhs.2 }
    return lhs.3 < rhs.3
}

func <= <A: Comparable, B: Comparable, C: Comparable, D: Comparable>(lhs: (A, B, C, D), rhs: (A, B, C, D)) -> Bool {
    if !(lhs.0 == rhs.0) { return lhs.0 < rhs.0 }
    if !(lhs.1 == rhs.1) { return lhs.1 < rhs.1 }
    if !(lhs.2 == rhs.2) { return lhs.2 < rhs.2 }
    return !(rhs.3 < lhs.3)
}

func > <A: Comparable, B: Comparable, C: Comparable, D: Comparable>(lhs: (A, B, C, D), rhs: (A, B, C, D)) -> Bool {
    if !(lhs.0 == rhs.0) { return rhs.0 < lhs.0 }
    if !(lhs.1 == rhs.1) { return rhs.1 < lhs.1 }
    if !(lhs.2 == rhs.2) { return rhs.2 < lhs.2 }
    return rhs.3 < lhs.3
}

func >= <A: Comparable, B: Comparable, C: Comparable, D: Comparable>(lhs: (A, B, C, D), rhs: (A, B, C, D)) -> Bool {
    if !(lhs.0 == rhs.0) { return rhs.0 < lhs.0 }
    if !(lhs.1 == rhs.1) { return rhs.1 < lhs.1 }
    if !(lhs.2 == rhs.2) { return rhs.2 < lhs.2 }
    return !(lhs.3 < rhs.3)
}

func == <A: Equatable, B: Equatable, C: Equatable, D: Equatable, E: Equatable>(lhs: (A, B, C, D, E), rhs: (A, B, C, D, E)) -> Bool {
    return lhs.0 == rhs.0 && lhs.1 == rhs.1 && lhs.2 == rhs.2 && lhs.3 == rhs.3 && lhs.4 == rhs.4
}

func != <A: Equatable, B: Equatable, C: Equatable, D: Equatable, E: Equatable>(lhs: (A, B, C, D, E), rhs: (A, B, C, D, E)) -> Bool {
    return !(lhs.0 == rhs.0) || !(lhs.1 == rhs.1) || !(lhs.2 == rhs.2) || !(lhs.3 == rhs.3) || !(lhs.4 == rhs.4)
}

func < <A: Comparable, B: Comparable, C: Comparable, D: Comparable, E: Comparable>(lhs: (A, B, C, D, E), rhs: (A, B, C, D, E)) -> Bool {
    if !(lhs.0 == rhs.0) { return lhs.0 < rhs.0 }
    if !(lhs.1 == rhs.1) { return lhs.1 < rhs.1 }
    if !(lhs.2 == rhs.2) { return lhs.2 < rhs.2 }
    if !(lhs.3 == rhs.3) { return lhs.3 < rhs.3 }
    return lhs.4 < rhs.4
}

func <= <A: Comparable, B: Comparable, C: Comparable, D: Comparable, E: Comparable>(lhs: (A, B, C, D, E), rhs: (A, B, C, D, E)) -> Bool {
    if !(lhs.0 == rhs.0) { return lhs.0 < rhs.0 }
    if !(lhs.1 == rhs.1) { return lhs.1 < rhs.1 }
    if !(lhs.2 == rhs.2) { return lhs.2 < rhs.2 }
    if !(lhs.3 == rhs.3) { return lhs.3 < rhs.3 }
    return !(rhs.4 < lhs.4)
}

func > <A: Comparable, B: Comparable, C: Comparable, D: Comparable, E: Comparable>(lhs: (A, B, C, D, E), rhs: (A, B, C, D, E)) -> Bool {
    if !(lhs.0 == rhs.0) { return rhs.0 < lhs.0 }
    if !(lhs.1 == rhs.1) { return rhs.1 < lhs.1 }
    if !(lhs.2 == rhs.2) { return rhs.2 < lhs.2 }
    if !(lhs.3 == rhs.3) { return rhs.3 < lhs.3 }
    return rhs.4 < lhs.4
}

func >= <A: Comparable, B: Comparable, C: Comparable, D: Comparable, E: Comparable>(lhs: (A, B, C, D, E), rhs: (A, B, C, D, E)) -> Bool {
    if !(lhs.0 == rhs.0) { return rhs.0 < lhs.0 }
    if !(lhs.1 == rhs.1) { return rhs.1 < lhs.1 }
    if !(lhs.2 == rhs.2) { return rhs.2 < lhs.2 }
    if !(lhs.3 == rhs.3) { return rhs.3 < lhs.3 }
    return !(lhs.4 < rhs.4)
}

func == <A: Equatable, B: Equatable, C: Equatable, D: Equatable, E: Equatable, F: Equatable>(lhs: (A, B, C, D, E, F), rhs: (A, B, C, D, E, F)) -> Bool {
    return lhs.0 == rhs.0 && lhs.1 == rhs.1 && lhs.2 == rhs.2 && lhs.3 == rhs.3 && lhs.4 == rhs.4 && lhs.5 == rhs.5
}

func != <A: Equatable, B: Equatable, C: Equatable, D: Equatable, E: Equatable, F: Equatable>(lhs: (A, B, C, D, E, F), rhs: (A, B, C, D, E, F)) -> Bool {
    return !(lhs.0 == rhs.0) || !(lhs.1 == rhs.1) || !(lhs.2 == rhs.2) || !(lhs.3 == rhs.3) || !(lhs.4 == rhs.4) || !(lhs.5 == rhs.5)
}

func < <A: Comparable, B: Comparable, C: Comparable, D: Comparable, E: Comparable, F: Comparable>(lhs: (A, B, C, D, E, F), rhs: (A, B, C, D, E, F)) -> Bool {
    if !(lhs.0 == rhs.0) { return lhs.0 < rhs.0 }
    if !(lhs.1 == rhs.1) { return lhs.1 < rhs.1 }
    if !(lhs.2 == rhs.2) { return lhs.2 < rhs.2 }
    if !(lhs.3 == rhs.3) { return lhs.3 < rhs.3 }
    if !(lhs.4 == rhs.4) { return lhs.4 < rhs.4 }
    return lhs.5 < rhs.5
}

func <= <A: Comparable, B: Comparable, C: Comparable, D: Comparable, E: Comparable, F: Comparable>(lhs: (A, B, C, D, E, F), rhs: (A, B, C, D, E, F)) -> Bool {
    if !(lhs.0 == rhs.0) { return lhs.0 < rhs.0 }
    if !(lhs.1 == rhs.1) { return lhs.1 < rhs.1 }
    if !(lhs.2 == rhs.2) { return lhs.2 < rhs.2 }
    if !(lhs.3 == rhs.3) { return lhs.3 < rhs.3 }
    if !(lhs.4 == rhs.4) { return lhs.4 < rhs.4 }
    return !(rhs.5 < lhs.5)
}

func > <A: Comparable, B: Comparable, C: Comparable, D: Comparable, E: Comparable, F: Comparable>(lhs: (A, B, C, D, E, F), rhs: (A, B, C, D, E, F)) -> Bool {
    if !(lhs.0 == rhs.0) { return rhs.0 < lhs.0 }
    if !(lhs.1 == rhs.1) { return rhs.1 < lhs.1 }
    if !(lhs.2 == rhs.2) { return rhs.2 < lhs.2 }
    if !(lhs.3 == rhs.3) { return rhs.3 < lhs.3 }
    if !(lhs.4 == rhs.4) { return rhs.4 < lhs.4 }
    return rhs.5 < lhs.5
}

func >= <A: Comparable, B: Comparable, C: Comparable, D: Comparable, E: Comparable, F: Comparable>(lhs: (A, B, C, D, E, F), rhs: (A, B, C, D, E, F)) -> Bool {
    if !(lhs.0 == rhs.0) { return rhs.0 < lhs.0 }
    if !(lhs.1 == rhs.1) { return rhs.1 < lhs.1 }
    if !(lhs.2 == rhs.2) { return rhs.2 < lhs.2 }
    if !(lhs.3 == rhs.3) { return rhs.3 < lhs.3 }
    if !(lhs.4 == rhs.4) { return rhs.4 < lhs.4 }
    return !(lhs.5 < rhs.5)
}

// ---- free functions ----

// The lesser of two values, the first where they are equal.
func min<T: Comparable>(_ x: T, _ y: T) -> T {
    return y < x ? y : x
}

// The greater of two values, the second where they are equal.
func max<T: Comparable>(_ x: T, _ y: T) -> T {
    return y >= x ? y : x
}

// The two sequences side by side, as pairs, as far as the shorter goes.
func zip<A, B>(_ first: [A], _ second: [B]) -> [(A, B)] {
    var out: [(A, B)] = []
    var i = 0
    while i < first.count && i < second.count {
        out.append((first[i], second[i]))
        i += 1
    }
    return out
}

// A sequence whose map and filter run only as its elements are asked
// for: Swift's lazy views of a collection, as one type. Its elements are
// what _at makes of the base's positions _lo..<_hi, where a filter has
// left one out _at answers nil. Walking its positions evaluates what
// Swift's LazyFilterCollection evaluates, so a closure runs as often.
struct LazySequence<Element>: Sequence {
    let _lo: Int
    let _hi: Int
    let _filtered: Bool
    let _at: (Int) -> Element?

    func makeIterator() -> _LazyIterator<Element> {
        return _LazyIterator(_seq: self, _i: _lo)
    }

    var lazy: LazySequence<Element> { return self }

    // What transform makes of each element, made as each is asked for.
    func map<T>(_ transform: @escaping (Element) -> T) -> LazySequence<T> {
        let at = _at
        return LazySequence<T>(_lo: _lo, _hi: _hi, _filtered: _filtered, _at: { (i: Int) -> T? in
            if let x = at(i) { return transform(x) }
            return nil
        })
    }

    // The elements isIncluded keeps, found as each is asked for.
    func filter(_ isIncluded: @escaping (Element) -> Bool) -> LazySequence<Element> {
        let at = _at
        return LazySequence<Element>(_lo: _lo, _hi: _hi, _filtered: true, _at: { (i: Int) -> Element? in
            if let x = at(i) {
                if isIncluded(x) { return x }
            }
            return nil
        })
    }

    // The first position holding an element.
    func _start() -> Int {
        if !_filtered { return _lo }
        var i = _lo
        while i < _hi {
            if let _ = _at(i) { break }
            i += 1
        }
        return i
    }

    // The position after i holding an element.
    func _after(_ i: Int) -> Int {
        if !_filtered { return i + 1 }
        var j = i + 1
        while j < _hi {
            if let _ = _at(j) { break }
            j += 1
        }
        return j
    }

    // At most the first maxLength elements.
    func prefix(_ maxLength: Int) -> LazySequence<Element> {
        let start = _start()
        var end = _start()
        var k = 0
        while k < maxLength && end < _hi {
            end = _after(end)
            k += 1
        }
        return LazySequence<Element>(_lo: start, _hi: end, _filtered: _filtered, _at: _at)
    }

    // The first element, or nil where there is none.
    var first: Element? {
        let i = _start()
        if i < _hi { return _at(i) }
        return nil
    }
}

struct _LazyIterator<Element>: IteratorProtocol {
    let _seq: LazySequence<Element>
    var _i: Int
    mutating func next() -> Element? {
        while _i < _seq._hi {
            let i = _i
            _i += 1
            if let x = _seq._at(i) { return x }
        }
        return nil
    }
}

extension Array {
    // The array as a lazy sequence.
    var lazy: LazySequence<Element> {
        let items = self
        return LazySequence<Element>(_lo: 0, _hi: count, _filtered: false, _at: { (i: Int) -> Element? in items[i] })
    }
}

extension Range where Bound == Int {
    // The integers as a lazy sequence.
    var lazy: LazySequence<Int> {
        return LazySequence<Int>(_lo: lowerBound, _hi: upperBound, _filtered: false, _at: { (i: Int) -> Int? in i })
    }
}

extension ClosedRange where Bound == Int {
    // The integers as a lazy sequence.
    var lazy: LazySequence<Int> {
        return LazySequence<Int>(_lo: lowerBound, _hi: upperBound + 1, _filtered: false, _at: { (i: Int) -> Int? in i })
    }
}

// The integers counted up from a start, beside the elements of an array.
func zip<B>(_ first: PartialRangeFrom<Int>, _ second: [B]) -> [(Int, B)] {
    var out: [(Int, B)] = []
    var i = 0
    while i < second.count {
        out.append((first.lowerBound + i, second[i]))
        i += 1
    }
    return out
}

// The elements of an array beside the integers counted up from a start.
func zip<A>(_ first: [A], _ second: PartialRangeFrom<Int>) -> [(A, Int)] {
    var out: [(A, Int)] = []
    var i = 0
    while i < first.count {
        out.append((first[i], second.lowerBound + i))
        i += 1
    }
    return out
}

// The values from start towards end in steps of stride, end left out.
func stride(from start: Int, to end: Int, by stride: Int) -> [Int] {
    var out: [Int] = []
    var x = start
    if stride > 0 {
        while x < end { out.append(x); x += stride }
    } else if stride < 0 {
        while x > end { out.append(x); x += stride }
    }
    return out
}

// The values from start towards end in steps of stride, end included
// where a step lands on it.
func stride(from start: Int, through end: Int, by stride: Int) -> [Int] {
    var out: [Int] = []
    var x = start
    if stride > 0 {
        while x <= end { out.append(x); x += stride }
    } else if stride < 0 {
        while x >= end { out.append(x); x += stride }
    }
    return out
}

func stride(from start: Double, to end: Double, by stride: Double) -> [Double] {
    var out: [Double] = []
    var i = 0.0
    if stride > 0 {
        while start + i * stride < end { out.append(start + i * stride); i += 1 }
    } else if stride < 0 {
        while start + i * stride > end { out.append(start + i * stride); i += 1 }
    }
    return out
}

func stride(from start: Double, through end: Double, by stride: Double) -> [Double] {
    var out: [Double] = []
    var i = 0.0
    if stride > 0 {
        while start + i * stride <= end { out.append(start + i * stride); i += 1 }
    } else if stride < 0 {
        while start + i * stride >= end { out.append(start + i * stride); i += 1 }
    }
    return out
}

// Exchanges two values.
func swap<T>(_ a: inout T, _ b: inout T) {
    let t = a
    a = b
    b = t
}

// ---- Integers ----

// init(bitPattern:) reads the other signedness's bits at the same width,
// and init(clamping:) is the nearest value the type has. Swift declares
// both generically over BinaryInteger; these take the types a program
// passes them, one width at a time.

extension Int8 {
    init(bitPattern x: UInt8) { self = Int8(truncatingIfNeeded: x) }
}

extension UInt8 {
    init(bitPattern x: Int8) { self = UInt8(truncatingIfNeeded: x) }
}

extension Int16 {
    init(bitPattern x: UInt16) { self = Int16(truncatingIfNeeded: x) }
}

extension UInt16 {
    init(bitPattern x: Int16) { self = UInt16(truncatingIfNeeded: x) }
}

extension Int32 {
    init(bitPattern x: UInt32) { self = Int32(truncatingIfNeeded: x) }
}

extension UInt32 {
    init(bitPattern x: Int32) { self = UInt32(truncatingIfNeeded: x) }
}

extension Int64 {
    init(bitPattern x: UInt64) { self = Int64(truncatingIfNeeded: x) }
}

extension UInt64 {
    init(bitPattern x: Int64) { self = UInt64(truncatingIfNeeded: x) }
}

extension Int {
    init(bitPattern x: UInt) { self = Int(truncatingIfNeeded: x) }
}

extension UInt {
    init(bitPattern x: Int) { self = UInt(truncatingIfNeeded: x) }
}

extension Int {
    init(clamping x: Int) { self = Int(x) }
}

extension Int8 {
    init(clamping x: Int) { self = x < Int(Int8.min) ? Int8.min : x > Int(Int8.max) ? Int8.max : Int8(x) }
}

extension Int16 {
    init(clamping x: Int) { self = x < Int(Int16.min) ? Int16.min : x > Int(Int16.max) ? Int16.max : Int16(x) }
}

extension Int32 {
    init(clamping x: Int) { self = x < Int(Int32.min) ? Int32.min : x > Int(Int32.max) ? Int32.max : Int32(x) }
}

extension Int64 {
    init(clamping x: Int) { self = Int64(x) }
}

extension UInt {
    init(clamping x: Int) { self = x < 0 ? 0 : UInt(x) }
}

extension UInt8 {
    init(clamping x: Int) { self = x < Int(UInt8.min) ? UInt8.min : x > Int(UInt8.max) ? UInt8.max : UInt8(x) }
}

extension UInt16 {
    init(clamping x: Int) { self = x < Int(UInt16.min) ? UInt16.min : x > Int(UInt16.max) ? UInt16.max : UInt16(x) }
}

extension UInt32 {
    init(clamping x: Int) { self = x < Int(UInt32.min) ? UInt32.min : x > Int(UInt32.max) ? UInt32.max : UInt32(x) }
}

extension UInt64 {
    init(clamping x: Int) { self = x < 0 ? 0 : UInt64(x) }
}

// init?(exactly:) is the same value, or nil where the type has no such
// value: out of range, or a Double with a fraction or no number at all.

extension Int {
    init?(exactly x: Int) {
        self = Int(x)
    }
    init?(exactly x: Double) {
        if !(x >= Double(Int.min) && x < -Double(Int.min)) { return nil }
        let v = Int(x)
        if Double(v) != x { return nil }
        self = v
    }
}

extension Int8 {
    init?(exactly x: Int) {
        if x < Int(Int8.min) || x > Int(Int8.max) { return nil }
        self = Int8(x)
    }
    init?(exactly x: Double) {
        if !(x >= Double(Int8.min) && x < -Double(Int8.min)) { return nil }
        let v = Int8(x)
        if Double(v) != x { return nil }
        self = v
    }
}

extension Int16 {
    init?(exactly x: Int) {
        if x < Int(Int16.min) || x > Int(Int16.max) { return nil }
        self = Int16(x)
    }
    init?(exactly x: Double) {
        if !(x >= Double(Int16.min) && x < -Double(Int16.min)) { return nil }
        let v = Int16(x)
        if Double(v) != x { return nil }
        self = v
    }
}

extension Int32 {
    init?(exactly x: Int) {
        if x < Int(Int32.min) || x > Int(Int32.max) { return nil }
        self = Int32(x)
    }
    init?(exactly x: Double) {
        if !(x >= Double(Int32.min) && x < -Double(Int32.min)) { return nil }
        let v = Int32(x)
        if Double(v) != x { return nil }
        self = v
    }
}

extension Int64 {
    init?(exactly x: Int) {
        self = Int64(x)
    }
    init?(exactly x: Double) {
        if !(x >= Double(Int64.min) && x < -Double(Int64.min)) { return nil }
        let v = Int64(x)
        if Double(v) != x { return nil }
        self = v
    }
}

extension UInt {
    init?(exactly x: Int) {
        if x < 0 { return nil }
        self = UInt(x)
    }
    init?(exactly x: Double) {
        if !(x >= 0 && x < Double(UInt.max) + 1) { return nil }
        let v = UInt(x)
        if Double(v) != x { return nil }
        self = v
    }
}

extension UInt8 {
    init?(exactly x: Int) {
        if x < Int(UInt8.min) || x > Int(UInt8.max) { return nil }
        self = UInt8(x)
    }
    init?(exactly x: Double) {
        if !(x >= 0 && x < Double(UInt8.max) + 1) { return nil }
        let v = UInt8(x)
        if Double(v) != x { return nil }
        self = v
    }
}

extension UInt16 {
    init?(exactly x: Int) {
        if x < Int(UInt16.min) || x > Int(UInt16.max) { return nil }
        self = UInt16(x)
    }
    init?(exactly x: Double) {
        if !(x >= 0 && x < Double(UInt16.max) + 1) { return nil }
        let v = UInt16(x)
        if Double(v) != x { return nil }
        self = v
    }
}

extension UInt32 {
    init?(exactly x: Int) {
        if x < Int(UInt32.min) || x > Int(UInt32.max) { return nil }
        self = UInt32(x)
    }
    init?(exactly x: Double) {
        if !(x >= 0 && x < Double(UInt32.max) + 1) { return nil }
        let v = UInt32(x)
        if Double(v) != x { return nil }
        self = v
    }
}

extension UInt64 {
    init?(exactly x: Int) {
        if x < 0 { return nil }
        self = UInt64(x)
    }
    init?(exactly x: Double) {
        if !(x >= 0 && x < Double(UInt64.max) + 1) { return nil }
        let v = UInt64(x)
        if Double(v) != x { return nil }
        self = v
    }
}

// ---- Bool ----

extension Bool {
    // Flips the value in place.
    mutating func toggle() { self = !self }

    // "true" or "false", exactly; nil for any other text.
    init?(_ description: String) {
        if description == "true" {
            self = true
        } else if description == "false" {
            self = false
        } else {
            return nil
        }
    }
}

// ---- Integer members ----

// What FixedWidthInteger and BinaryInteger give each integer type in Swift,
// written once per type over the instructions core.swift declares.

extension Int {
    static var isSigned: Bool { true }
    var bitWidth: Int { 64 }
    var nonzeroBitCount: Int { Int(_popcount(self)) }
    var leadingZeroBitCount: Int { Int(_leadingZeros(self)) }
    var trailingZeroBitCount: Int { Int(_trailingZeros(self)) }
    var byteSwapped: Int { _byteSwapped(self) }
    // The value's words, least significant first: one, for these widths.
    var words: [UInt] { [UInt(truncatingIfNeeded: self)] }
    var magnitude: UInt { self < 0 ? UInt(truncatingIfNeeded: 0 &- self) : UInt(self) }

    func signum() -> Int { self > 0 ? 1 : self < 0 ? -1 : 0 }

    func addingReportingOverflow(_ rhs: Int) -> (partialValue: Int, overflow: Bool) {
        let r = _addingReportingOverflow(self, rhs)
        return (partialValue: r.0, overflow: r.1)
    }
    func subtractingReportingOverflow(_ rhs: Int) -> (partialValue: Int, overflow: Bool) {
        let r = _subtractingReportingOverflow(self, rhs)
        return (partialValue: r.0, overflow: r.1)
    }
    func multipliedReportingOverflow(by rhs: Int) -> (partialValue: Int, overflow: Bool) {
        let r = _multipliedReportingOverflow(self, rhs)
        return (partialValue: r.0, overflow: r.1)
    }
    func dividedReportingOverflow(by rhs: Int) -> (partialValue: Int, overflow: Bool) {
        if rhs == 0 { return (self, true) }
        if self == Int.min && rhs == -1 { return (self, true) }
        return (partialValue: self / rhs, overflow: false)
    }
    func remainderReportingOverflow(dividingBy rhs: Int) -> (partialValue: Int, overflow: Bool) {
        if rhs == 0 { return (self, true) }
        if self == Int.min && rhs == -1 { return (0, true) }
        return (partialValue: self % rhs, overflow: false)
    }
    func multipliedFullWidth(by other: Int) -> (high: Int, low: UInt) {
        return (high: _multipliedHigh(self, other), low: UInt(truncatingIfNeeded: self &* other))
    }
    func quotientAndRemainder(dividingBy rhs: Int) -> (quotient: Int, remainder: Int) {
        (quotient: self / rhs, remainder: self % rhs)
    }
    func isMultiple(of other: Int) -> Bool {
        if other == 0 { return self == 0 }
        if other == -1 { return true }
        return self % other == 0
    }
    func distance(to other: Int) -> Int { Int(other) - Int(self) }
    func advanced(by n: Int) -> Int { Int(Int(self) + n) }
}

func abs(_ x: Int) -> Int { x < 0 ? -x : x }

extension Int8 {
    static var isSigned: Bool { true }
    var bitWidth: Int { 8 }
    var nonzeroBitCount: Int { Int(_popcount(self)) }
    var leadingZeroBitCount: Int { Int(_leadingZeros(self)) }
    var trailingZeroBitCount: Int { Int(_trailingZeros(self)) }
    var byteSwapped: Int8 { _byteSwapped(self) }
    // The value's words, least significant first: one, for these widths.
    var words: [UInt] { [UInt(truncatingIfNeeded: self)] }
    var magnitude: UInt8 { self < 0 ? UInt8(truncatingIfNeeded: 0 &- self) : UInt8(self) }

    func signum() -> Int8 { self > 0 ? 1 : self < 0 ? -1 : 0 }

    func addingReportingOverflow(_ rhs: Int8) -> (partialValue: Int8, overflow: Bool) {
        let r = _addingReportingOverflow(self, rhs)
        return (partialValue: r.0, overflow: r.1)
    }
    func subtractingReportingOverflow(_ rhs: Int8) -> (partialValue: Int8, overflow: Bool) {
        let r = _subtractingReportingOverflow(self, rhs)
        return (partialValue: r.0, overflow: r.1)
    }
    func multipliedReportingOverflow(by rhs: Int8) -> (partialValue: Int8, overflow: Bool) {
        let r = _multipliedReportingOverflow(self, rhs)
        return (partialValue: r.0, overflow: r.1)
    }
    func dividedReportingOverflow(by rhs: Int8) -> (partialValue: Int8, overflow: Bool) {
        if rhs == 0 { return (self, true) }
        if self == Int8.min && rhs == -1 { return (self, true) }
        return (partialValue: self / rhs, overflow: false)
    }
    func remainderReportingOverflow(dividingBy rhs: Int8) -> (partialValue: Int8, overflow: Bool) {
        if rhs == 0 { return (self, true) }
        if self == Int8.min && rhs == -1 { return (0, true) }
        return (partialValue: self % rhs, overflow: false)
    }
    func multipliedFullWidth(by other: Int8) -> (high: Int8, low: UInt8) {
        let p = Int64(self) * Int64(other)
        return (high: Int8(truncatingIfNeeded: p >> 8), low: UInt8(truncatingIfNeeded: p))
    }
    func quotientAndRemainder(dividingBy rhs: Int8) -> (quotient: Int8, remainder: Int8) {
        (quotient: self / rhs, remainder: self % rhs)
    }
    func isMultiple(of other: Int8) -> Bool {
        if other == 0 { return self == 0 }
        if other == -1 { return true }
        return self % other == 0
    }
    func distance(to other: Int8) -> Int { Int(other) - Int(self) }
    func advanced(by n: Int) -> Int8 { Int8(Int(self) + n) }
}

func abs(_ x: Int8) -> Int8 { x < 0 ? -x : x }

extension Int16 {
    static var isSigned: Bool { true }
    var bitWidth: Int { 16 }
    var nonzeroBitCount: Int { Int(_popcount(self)) }
    var leadingZeroBitCount: Int { Int(_leadingZeros(self)) }
    var trailingZeroBitCount: Int { Int(_trailingZeros(self)) }
    var byteSwapped: Int16 { _byteSwapped(self) }
    // The value's words, least significant first: one, for these widths.
    var words: [UInt] { [UInt(truncatingIfNeeded: self)] }
    var magnitude: UInt16 { self < 0 ? UInt16(truncatingIfNeeded: 0 &- self) : UInt16(self) }

    func signum() -> Int16 { self > 0 ? 1 : self < 0 ? -1 : 0 }

    func addingReportingOverflow(_ rhs: Int16) -> (partialValue: Int16, overflow: Bool) {
        let r = _addingReportingOverflow(self, rhs)
        return (partialValue: r.0, overflow: r.1)
    }
    func subtractingReportingOverflow(_ rhs: Int16) -> (partialValue: Int16, overflow: Bool) {
        let r = _subtractingReportingOverflow(self, rhs)
        return (partialValue: r.0, overflow: r.1)
    }
    func multipliedReportingOverflow(by rhs: Int16) -> (partialValue: Int16, overflow: Bool) {
        let r = _multipliedReportingOverflow(self, rhs)
        return (partialValue: r.0, overflow: r.1)
    }
    func dividedReportingOverflow(by rhs: Int16) -> (partialValue: Int16, overflow: Bool) {
        if rhs == 0 { return (self, true) }
        if self == Int16.min && rhs == -1 { return (self, true) }
        return (partialValue: self / rhs, overflow: false)
    }
    func remainderReportingOverflow(dividingBy rhs: Int16) -> (partialValue: Int16, overflow: Bool) {
        if rhs == 0 { return (self, true) }
        if self == Int16.min && rhs == -1 { return (0, true) }
        return (partialValue: self % rhs, overflow: false)
    }
    func multipliedFullWidth(by other: Int16) -> (high: Int16, low: UInt16) {
        let p = Int64(self) * Int64(other)
        return (high: Int16(truncatingIfNeeded: p >> 16), low: UInt16(truncatingIfNeeded: p))
    }
    func quotientAndRemainder(dividingBy rhs: Int16) -> (quotient: Int16, remainder: Int16) {
        (quotient: self / rhs, remainder: self % rhs)
    }
    func isMultiple(of other: Int16) -> Bool {
        if other == 0 { return self == 0 }
        if other == -1 { return true }
        return self % other == 0
    }
    func distance(to other: Int16) -> Int { Int(other) - Int(self) }
    func advanced(by n: Int) -> Int16 { Int16(Int(self) + n) }
}

func abs(_ x: Int16) -> Int16 { x < 0 ? -x : x }

extension Int32 {
    static var isSigned: Bool { true }
    var bitWidth: Int { 32 }
    var nonzeroBitCount: Int { Int(_popcount(self)) }
    var leadingZeroBitCount: Int { Int(_leadingZeros(self)) }
    var trailingZeroBitCount: Int { Int(_trailingZeros(self)) }
    var byteSwapped: Int32 { _byteSwapped(self) }
    // The value's words, least significant first: one, for these widths.
    var words: [UInt] { [UInt(truncatingIfNeeded: self)] }
    var magnitude: UInt32 { self < 0 ? UInt32(truncatingIfNeeded: 0 &- self) : UInt32(self) }

    func signum() -> Int32 { self > 0 ? 1 : self < 0 ? -1 : 0 }

    func addingReportingOverflow(_ rhs: Int32) -> (partialValue: Int32, overflow: Bool) {
        let r = _addingReportingOverflow(self, rhs)
        return (partialValue: r.0, overflow: r.1)
    }
    func subtractingReportingOverflow(_ rhs: Int32) -> (partialValue: Int32, overflow: Bool) {
        let r = _subtractingReportingOverflow(self, rhs)
        return (partialValue: r.0, overflow: r.1)
    }
    func multipliedReportingOverflow(by rhs: Int32) -> (partialValue: Int32, overflow: Bool) {
        let r = _multipliedReportingOverflow(self, rhs)
        return (partialValue: r.0, overflow: r.1)
    }
    func dividedReportingOverflow(by rhs: Int32) -> (partialValue: Int32, overflow: Bool) {
        if rhs == 0 { return (self, true) }
        if self == Int32.min && rhs == -1 { return (self, true) }
        return (partialValue: self / rhs, overflow: false)
    }
    func remainderReportingOverflow(dividingBy rhs: Int32) -> (partialValue: Int32, overflow: Bool) {
        if rhs == 0 { return (self, true) }
        if self == Int32.min && rhs == -1 { return (0, true) }
        return (partialValue: self % rhs, overflow: false)
    }
    func multipliedFullWidth(by other: Int32) -> (high: Int32, low: UInt32) {
        let p = Int64(self) * Int64(other)
        return (high: Int32(truncatingIfNeeded: p >> 32), low: UInt32(truncatingIfNeeded: p))
    }
    func quotientAndRemainder(dividingBy rhs: Int32) -> (quotient: Int32, remainder: Int32) {
        (quotient: self / rhs, remainder: self % rhs)
    }
    func isMultiple(of other: Int32) -> Bool {
        if other == 0 { return self == 0 }
        if other == -1 { return true }
        return self % other == 0
    }
    func distance(to other: Int32) -> Int { Int(other) - Int(self) }
    func advanced(by n: Int) -> Int32 { Int32(Int(self) + n) }
}

func abs(_ x: Int32) -> Int32 { x < 0 ? -x : x }

extension Int64 {
    static var isSigned: Bool { true }
    var bitWidth: Int { 64 }
    var nonzeroBitCount: Int { Int(_popcount(self)) }
    var leadingZeroBitCount: Int { Int(_leadingZeros(self)) }
    var trailingZeroBitCount: Int { Int(_trailingZeros(self)) }
    var byteSwapped: Int64 { _byteSwapped(self) }
    // The value's words, least significant first: one, for these widths.
    var words: [UInt] { [UInt(truncatingIfNeeded: self)] }
    var magnitude: UInt64 { self < 0 ? UInt64(truncatingIfNeeded: 0 &- self) : UInt64(self) }

    func signum() -> Int64 { self > 0 ? 1 : self < 0 ? -1 : 0 }

    func addingReportingOverflow(_ rhs: Int64) -> (partialValue: Int64, overflow: Bool) {
        let r = _addingReportingOverflow(self, rhs)
        return (partialValue: r.0, overflow: r.1)
    }
    func subtractingReportingOverflow(_ rhs: Int64) -> (partialValue: Int64, overflow: Bool) {
        let r = _subtractingReportingOverflow(self, rhs)
        return (partialValue: r.0, overflow: r.1)
    }
    func multipliedReportingOverflow(by rhs: Int64) -> (partialValue: Int64, overflow: Bool) {
        let r = _multipliedReportingOverflow(self, rhs)
        return (partialValue: r.0, overflow: r.1)
    }
    func dividedReportingOverflow(by rhs: Int64) -> (partialValue: Int64, overflow: Bool) {
        if rhs == 0 { return (self, true) }
        if self == Int64.min && rhs == -1 { return (self, true) }
        return (partialValue: self / rhs, overflow: false)
    }
    func remainderReportingOverflow(dividingBy rhs: Int64) -> (partialValue: Int64, overflow: Bool) {
        if rhs == 0 { return (self, true) }
        if self == Int64.min && rhs == -1 { return (0, true) }
        return (partialValue: self % rhs, overflow: false)
    }
    func multipliedFullWidth(by other: Int64) -> (high: Int64, low: UInt64) {
        return (high: _multipliedHigh(self, other), low: UInt64(truncatingIfNeeded: self &* other))
    }
    func quotientAndRemainder(dividingBy rhs: Int64) -> (quotient: Int64, remainder: Int64) {
        (quotient: self / rhs, remainder: self % rhs)
    }
    func isMultiple(of other: Int64) -> Bool {
        if other == 0 { return self == 0 }
        if other == -1 { return true }
        return self % other == 0
    }
    func distance(to other: Int64) -> Int { Int(other) - Int(self) }
    func advanced(by n: Int) -> Int64 { Int64(Int(self) + n) }
}

func abs(_ x: Int64) -> Int64 { x < 0 ? -x : x }

extension UInt {
    static var isSigned: Bool { false }
    var bitWidth: Int { 64 }
    var nonzeroBitCount: Int { Int(_popcount(self)) }
    var leadingZeroBitCount: Int { Int(_leadingZeros(self)) }
    var trailingZeroBitCount: Int { Int(_trailingZeros(self)) }
    var byteSwapped: UInt { _byteSwapped(self) }
    // The value's words, least significant first: one, for these widths.
    var words: [UInt] { [UInt(truncatingIfNeeded: self)] }
    var magnitude: UInt { self }

    func signum() -> UInt { self == 0 ? 0 : 1 }

    func addingReportingOverflow(_ rhs: UInt) -> (partialValue: UInt, overflow: Bool) {
        let r = _addingReportingOverflow(self, rhs)
        return (partialValue: r.0, overflow: r.1)
    }
    func subtractingReportingOverflow(_ rhs: UInt) -> (partialValue: UInt, overflow: Bool) {
        let r = _subtractingReportingOverflow(self, rhs)
        return (partialValue: r.0, overflow: r.1)
    }
    func multipliedReportingOverflow(by rhs: UInt) -> (partialValue: UInt, overflow: Bool) {
        let r = _multipliedReportingOverflow(self, rhs)
        return (partialValue: r.0, overflow: r.1)
    }
    func dividedReportingOverflow(by rhs: UInt) -> (partialValue: UInt, overflow: Bool) {
        if rhs == 0 { return (self, true) }
        return (partialValue: self / rhs, overflow: false)
    }
    func remainderReportingOverflow(dividingBy rhs: UInt) -> (partialValue: UInt, overflow: Bool) {
        if rhs == 0 { return (self, true) }
        return (partialValue: self % rhs, overflow: false)
    }
    func multipliedFullWidth(by other: UInt) -> (high: UInt, low: UInt) {
        return (high: _multipliedHigh(self, other), low: UInt(truncatingIfNeeded: self &* other))
    }
    func quotientAndRemainder(dividingBy rhs: UInt) -> (quotient: UInt, remainder: UInt) {
        (quotient: self / rhs, remainder: self % rhs)
    }
    func isMultiple(of other: UInt) -> Bool {
        if other == 0 { return self == 0 }
        return self % other == 0
    }
    func distance(to other: UInt) -> Int { Int(other) - Int(self) }
    func advanced(by n: Int) -> UInt { UInt(Int(self) + n) }
}

extension UInt8 {
    static var isSigned: Bool { false }
    var bitWidth: Int { 8 }
    var nonzeroBitCount: Int { Int(_popcount(self)) }
    var leadingZeroBitCount: Int { Int(_leadingZeros(self)) }
    var trailingZeroBitCount: Int { Int(_trailingZeros(self)) }
    var byteSwapped: UInt8 { _byteSwapped(self) }
    // The value's words, least significant first: one, for these widths.
    var words: [UInt] { [UInt(truncatingIfNeeded: self)] }
    var magnitude: UInt8 { self }

    func signum() -> UInt8 { self == 0 ? 0 : 1 }

    func addingReportingOverflow(_ rhs: UInt8) -> (partialValue: UInt8, overflow: Bool) {
        let r = _addingReportingOverflow(self, rhs)
        return (partialValue: r.0, overflow: r.1)
    }
    func subtractingReportingOverflow(_ rhs: UInt8) -> (partialValue: UInt8, overflow: Bool) {
        let r = _subtractingReportingOverflow(self, rhs)
        return (partialValue: r.0, overflow: r.1)
    }
    func multipliedReportingOverflow(by rhs: UInt8) -> (partialValue: UInt8, overflow: Bool) {
        let r = _multipliedReportingOverflow(self, rhs)
        return (partialValue: r.0, overflow: r.1)
    }
    func dividedReportingOverflow(by rhs: UInt8) -> (partialValue: UInt8, overflow: Bool) {
        if rhs == 0 { return (self, true) }
        return (partialValue: self / rhs, overflow: false)
    }
    func remainderReportingOverflow(dividingBy rhs: UInt8) -> (partialValue: UInt8, overflow: Bool) {
        if rhs == 0 { return (self, true) }
        return (partialValue: self % rhs, overflow: false)
    }
    func multipliedFullWidth(by other: UInt8) -> (high: UInt8, low: UInt8) {
        let p = UInt64(self) * UInt64(other)
        return (high: UInt8(truncatingIfNeeded: p >> 8), low: UInt8(truncatingIfNeeded: p))
    }
    func quotientAndRemainder(dividingBy rhs: UInt8) -> (quotient: UInt8, remainder: UInt8) {
        (quotient: self / rhs, remainder: self % rhs)
    }
    func isMultiple(of other: UInt8) -> Bool {
        if other == 0 { return self == 0 }
        return self % other == 0
    }
    func distance(to other: UInt8) -> Int { Int(other) - Int(self) }
    func advanced(by n: Int) -> UInt8 { UInt8(Int(self) + n) }
}

extension UInt16 {
    static var isSigned: Bool { false }
    var bitWidth: Int { 16 }
    var nonzeroBitCount: Int { Int(_popcount(self)) }
    var leadingZeroBitCount: Int { Int(_leadingZeros(self)) }
    var trailingZeroBitCount: Int { Int(_trailingZeros(self)) }
    var byteSwapped: UInt16 { _byteSwapped(self) }
    // The value's words, least significant first: one, for these widths.
    var words: [UInt] { [UInt(truncatingIfNeeded: self)] }
    var magnitude: UInt16 { self }

    func signum() -> UInt16 { self == 0 ? 0 : 1 }

    func addingReportingOverflow(_ rhs: UInt16) -> (partialValue: UInt16, overflow: Bool) {
        let r = _addingReportingOverflow(self, rhs)
        return (partialValue: r.0, overflow: r.1)
    }
    func subtractingReportingOverflow(_ rhs: UInt16) -> (partialValue: UInt16, overflow: Bool) {
        let r = _subtractingReportingOverflow(self, rhs)
        return (partialValue: r.0, overflow: r.1)
    }
    func multipliedReportingOverflow(by rhs: UInt16) -> (partialValue: UInt16, overflow: Bool) {
        let r = _multipliedReportingOverflow(self, rhs)
        return (partialValue: r.0, overflow: r.1)
    }
    func dividedReportingOverflow(by rhs: UInt16) -> (partialValue: UInt16, overflow: Bool) {
        if rhs == 0 { return (self, true) }
        return (partialValue: self / rhs, overflow: false)
    }
    func remainderReportingOverflow(dividingBy rhs: UInt16) -> (partialValue: UInt16, overflow: Bool) {
        if rhs == 0 { return (self, true) }
        return (partialValue: self % rhs, overflow: false)
    }
    func multipliedFullWidth(by other: UInt16) -> (high: UInt16, low: UInt16) {
        let p = UInt64(self) * UInt64(other)
        return (high: UInt16(truncatingIfNeeded: p >> 16), low: UInt16(truncatingIfNeeded: p))
    }
    func quotientAndRemainder(dividingBy rhs: UInt16) -> (quotient: UInt16, remainder: UInt16) {
        (quotient: self / rhs, remainder: self % rhs)
    }
    func isMultiple(of other: UInt16) -> Bool {
        if other == 0 { return self == 0 }
        return self % other == 0
    }
    func distance(to other: UInt16) -> Int { Int(other) - Int(self) }
    func advanced(by n: Int) -> UInt16 { UInt16(Int(self) + n) }
}

extension UInt32 {
    static var isSigned: Bool { false }
    var bitWidth: Int { 32 }
    var nonzeroBitCount: Int { Int(_popcount(self)) }
    var leadingZeroBitCount: Int { Int(_leadingZeros(self)) }
    var trailingZeroBitCount: Int { Int(_trailingZeros(self)) }
    var byteSwapped: UInt32 { _byteSwapped(self) }
    // The value's words, least significant first: one, for these widths.
    var words: [UInt] { [UInt(truncatingIfNeeded: self)] }
    var magnitude: UInt32 { self }

    func signum() -> UInt32 { self == 0 ? 0 : 1 }

    func addingReportingOverflow(_ rhs: UInt32) -> (partialValue: UInt32, overflow: Bool) {
        let r = _addingReportingOverflow(self, rhs)
        return (partialValue: r.0, overflow: r.1)
    }
    func subtractingReportingOverflow(_ rhs: UInt32) -> (partialValue: UInt32, overflow: Bool) {
        let r = _subtractingReportingOverflow(self, rhs)
        return (partialValue: r.0, overflow: r.1)
    }
    func multipliedReportingOverflow(by rhs: UInt32) -> (partialValue: UInt32, overflow: Bool) {
        let r = _multipliedReportingOverflow(self, rhs)
        return (partialValue: r.0, overflow: r.1)
    }
    func dividedReportingOverflow(by rhs: UInt32) -> (partialValue: UInt32, overflow: Bool) {
        if rhs == 0 { return (self, true) }
        return (partialValue: self / rhs, overflow: false)
    }
    func remainderReportingOverflow(dividingBy rhs: UInt32) -> (partialValue: UInt32, overflow: Bool) {
        if rhs == 0 { return (self, true) }
        return (partialValue: self % rhs, overflow: false)
    }
    func multipliedFullWidth(by other: UInt32) -> (high: UInt32, low: UInt32) {
        let p = UInt64(self) * UInt64(other)
        return (high: UInt32(truncatingIfNeeded: p >> 32), low: UInt32(truncatingIfNeeded: p))
    }
    func quotientAndRemainder(dividingBy rhs: UInt32) -> (quotient: UInt32, remainder: UInt32) {
        (quotient: self / rhs, remainder: self % rhs)
    }
    func isMultiple(of other: UInt32) -> Bool {
        if other == 0 { return self == 0 }
        return self % other == 0
    }
    func distance(to other: UInt32) -> Int { Int(other) - Int(self) }
    func advanced(by n: Int) -> UInt32 { UInt32(Int(self) + n) }
}

extension UInt64 {
    static var isSigned: Bool { false }
    var bitWidth: Int { 64 }
    var nonzeroBitCount: Int { Int(_popcount(self)) }
    var leadingZeroBitCount: Int { Int(_leadingZeros(self)) }
    var trailingZeroBitCount: Int { Int(_trailingZeros(self)) }
    var byteSwapped: UInt64 { _byteSwapped(self) }
    // The value's words, least significant first: one, for these widths.
    var words: [UInt] { [UInt(truncatingIfNeeded: self)] }
    var magnitude: UInt64 { self }

    func signum() -> UInt64 { self == 0 ? 0 : 1 }

    func addingReportingOverflow(_ rhs: UInt64) -> (partialValue: UInt64, overflow: Bool) {
        let r = _addingReportingOverflow(self, rhs)
        return (partialValue: r.0, overflow: r.1)
    }
    func subtractingReportingOverflow(_ rhs: UInt64) -> (partialValue: UInt64, overflow: Bool) {
        let r = _subtractingReportingOverflow(self, rhs)
        return (partialValue: r.0, overflow: r.1)
    }
    func multipliedReportingOverflow(by rhs: UInt64) -> (partialValue: UInt64, overflow: Bool) {
        let r = _multipliedReportingOverflow(self, rhs)
        return (partialValue: r.0, overflow: r.1)
    }
    func dividedReportingOverflow(by rhs: UInt64) -> (partialValue: UInt64, overflow: Bool) {
        if rhs == 0 { return (self, true) }
        return (partialValue: self / rhs, overflow: false)
    }
    func remainderReportingOverflow(dividingBy rhs: UInt64) -> (partialValue: UInt64, overflow: Bool) {
        if rhs == 0 { return (self, true) }
        return (partialValue: self % rhs, overflow: false)
    }
    func multipliedFullWidth(by other: UInt64) -> (high: UInt64, low: UInt64) {
        return (high: _multipliedHigh(self, other), low: UInt64(truncatingIfNeeded: self &* other))
    }
    func quotientAndRemainder(dividingBy rhs: UInt64) -> (quotient: UInt64, remainder: UInt64) {
        (quotient: self / rhs, remainder: self % rhs)
    }
    func isMultiple(of other: UInt64) -> Bool {
        if other == 0 { return self == 0 }
        return self % other == 0
    }
    func distance(to other: UInt64) -> Int { Int(other) - Int(self) }
    func advanced(by n: Int) -> UInt64 { UInt64(Int(self) + n) }
}

// ---- Floating point ----

// Which side of zero a value is on.
enum FloatingPointSign: Int {
    case plus = 0, minus = 1
}

extension Double {
    static var pi: Double { 3.141592653589793 }
    static var infinity: Double { 1.0 / Double(0) }
    static var nan: Double { Double(0) / Double(0) }
    static var greatestFiniteMagnitude: Double { 1.7976931348623157e308 }
    static var leastNonzeroMagnitude: Double { 5e-324 }
    static var leastNormalMagnitude: Double { 2.2250738585072014e-308 }
    static var ulpOfOne: Double { 2.220446049250313e-16 }

    init(bitPattern bits: UInt64) { self = _double(bits) }
    var bitPattern: UInt64 { _bits(self) }

    // NaN is the one value unequal to itself.
    var isNaN: Bool { self != self }
    var isInfinite: Bool { _fabs(self) == Double.infinity }
    var isFinite: Bool { !isNaN && !isInfinite }
    var isZero: Bool { self == 0 }
    var isNormal: Bool {
        let e = (bitPattern >> 52) & 0x7FF
        return e != 0 && e != 0x7FF
    }
    var isSubnormal: Bool { (bitPattern >> 52) & 0x7FF == 0 && bitPattern & 0xF_FFFF_FFFF_FFFF != 0 }
    var magnitude: Double { _fabs(self) }
    var sign: FloatingPointSign { bitPattern >> 63 == 0 ? .plus : .minus }
    var isSignMinus: Bool { bitPattern >> 63 != 0 }

    func squareRoot() -> Double { _sqrt(self) }
    mutating func formSquareRoot() { self = _sqrt(self) }

    func rounded(_ rule: FloatingPointRoundingRule) -> Double {
        switch rule {
        case .toNearestOrAwayFromZero:
            // Away from zero at the half: 2.5 is 3 and -2.5 is -3.
            let t = _trunc(self)
            if _fabs(self - t) >= 0.5 { return t + _copysign(1, self) }
            return t
        case .toNearestOrEven:
            return _rint(self)
        case .up:
            return _ceil(self)
        case .down:
            return _floor(self)
        case .towardZero:
            return _trunc(self)
        case .awayFromZero:
            let t = _trunc(self)
            return t == self ? self : t + _copysign(1, self)
        }
    }
    func rounded() -> Double { rounded(.toNearestOrAwayFromZero) }
    mutating func round() { self = rounded() }
    mutating func round(_ rule: FloatingPointRoundingRule) { self = rounded(rule) }

    // The power of two the value lies above: Int.min for zero, Int.max
    // for an infinity or NaN, as Swift answers.
    var exponent: Int {
        if !isFinite { return Int.max }
        if self == 0 { return Int.min }
        let e = Int((bitPattern >> 52) & 0x7FF)
        if e != 0 { return e - 1023 }
        let m = bitPattern & 0xF_FFFF_FFFF_FFFF
        return (63 - m.leadingZeroBitCount) + 1 - 1023 - 52
    }
    // The value scaled into [1, 2), keeping its sign out.
    var significand: Double {
        if isNaN { return self }
        if isInfinite { return Double.infinity }
        if self == 0 { return 0 }
        var m = bitPattern & 0xF_FFFF_FFFF_FFFF
        if (bitPattern >> 52) & 0x7FF == 0 {
            m = (m << UInt64(m.leadingZeroBitCount - 11)) & 0xF_FFFF_FFFF_FFFF
        }
        return Double(bitPattern: m | (UInt64(1023) << 52))
    }

    var nextUp: Double {
        if isNaN || self == Double.infinity { return self }
        if self == 0 { return Double.leastNonzeroMagnitude }
        return Double(bitPattern: self > 0 ? bitPattern + 1 : bitPattern - 1)
    }
    var nextDown: Double { -(-self).nextUp }
    var ulp: Double {
        if !isFinite { return Double.nan }
        let a = _fabs(self)
        if a == Double.greatestFiniteMagnitude { return a - a.nextDown }
        return a.nextUp - a
    }

    func truncatingRemainder(dividingBy other: Double) -> Double { _fmod(self, other) }
    mutating func formTruncatingRemainder(dividingBy other: Double) { self = _fmod(self, other) }

    // IEEE 754's remainder: self minus other times the nearest integer to
    // their quotient, ties to even. Exact, as every step below is.
    func remainder(dividingBy other: Double) -> Double {
        if isNaN || other.isNaN || isInfinite || other == 0 { return Double.nan }
        if other.isInfinite { return self }
        let m = _fabs(other)
        var a = _fabs(self)
        var odd = false
        if (m + m).isFinite { a = _fmod(a, m + m) }
        if a >= m {
            a = a - m
            odd = true
        }
        if a + a > m || (a + a == m && odd) { a = a - m }
        return isSignMinus ? -a : a
    }
    mutating func formRemainder(dividingBy other: Double) { self = remainder(dividingBy: other) }
}

// fmod: x less the whole multiples of y that fit, exactly. d runs down
// the powers of two times |y| no larger than |x|, and each subtraction is
// of numbers within a factor of two of each other, which is exact.
func _fmod(_ x: Double, _ y: Double) -> Double {
    if x.isNaN || y.isNaN || x.isInfinite || y == 0 { return Double.nan }
    if y.isInfinite { return x }
    var r = _fabs(x)
    let m = _fabs(y)
    var d = m
    while d + d <= r { d = d + d }
    while d >= m {
        if r >= d { r = r - d }
        d = d * 0.5
    }
    return _copysign(r, x)
}

func abs(_ x: Double) -> Double { _fabs(x) }

extension Float {
    static var pi: Float { 3.1415925 }
    static var infinity: Float { 1.0 / Float(0) }
    static var nan: Float { Float(0) / Float(0) }
    static var greatestFiniteMagnitude: Float { 3.4028235e38 }
    static var leastNonzeroMagnitude: Float { 1e-45 }
    static var leastNormalMagnitude: Float { 1.17549435e-38 }
    static var ulpOfOne: Float { 1.1920929e-07 }

    init(bitPattern bits: UInt32) { self = _float(bits) }
    var bitPattern: UInt32 { _bits(self) }

    // NaN is the one value unequal to itself.
    var isNaN: Bool { self != self }
    var isInfinite: Bool { _fabs(self) == Float.infinity }
    var isFinite: Bool { !isNaN && !isInfinite }
    var isZero: Bool { self == 0 }
    var isNormal: Bool {
        let e = (bitPattern >> 23) & 0xFF
        return e != 0 && e != 0xFF
    }
    var isSubnormal: Bool { (bitPattern >> 23) & 0xFF == 0 && bitPattern & 0x7F_FFFF != 0 }
    var magnitude: Float { _fabs(self) }
    var sign: FloatingPointSign { bitPattern >> 31 == 0 ? .plus : .minus }
    var isSignMinus: Bool { bitPattern >> 31 != 0 }

    func squareRoot() -> Float { _sqrt(self) }
    mutating func formSquareRoot() { self = _sqrt(self) }

    func rounded(_ rule: FloatingPointRoundingRule) -> Float {
        switch rule {
        case .toNearestOrAwayFromZero:
            // Away from zero at the half: 2.5 is 3 and -2.5 is -3.
            let t = _trunc(self)
            if _fabs(self - t) >= 0.5 { return t + _copysign(1, self) }
            return t
        case .toNearestOrEven:
            return _rint(self)
        case .up:
            return _ceil(self)
        case .down:
            return _floor(self)
        case .towardZero:
            return _trunc(self)
        case .awayFromZero:
            let t = _trunc(self)
            return t == self ? self : t + _copysign(1, self)
        }
    }
    func rounded() -> Float { rounded(.toNearestOrAwayFromZero) }
    mutating func round() { self = rounded() }
    mutating func round(_ rule: FloatingPointRoundingRule) { self = rounded(rule) }

    // The power of two the value lies above: Int.min for zero, Int.max
    // for an infinity or NaN, as Swift answers.
    var exponent: Int {
        if !isFinite { return Int.max }
        if self == 0 { return Int.min }
        let e = Int((bitPattern >> 23) & 0xFF)
        if e != 0 { return e - 127 }
        let m = bitPattern & 0x7F_FFFF
        return (31 - m.leadingZeroBitCount) + 1 - 127 - 23
    }
    // The value scaled into [1, 2), keeping its sign out.
    var significand: Float {
        if isNaN { return self }
        if isInfinite { return Float.infinity }
        if self == 0 { return 0 }
        var m = bitPattern & 0x7F_FFFF
        if (bitPattern >> 23) & 0xFF == 0 {
            m = (m << UInt32(m.leadingZeroBitCount - 8)) & 0x7F_FFFF
        }
        return Float(bitPattern: m | (UInt32(127) << 23))
    }

    var nextUp: Float {
        if isNaN || self == Float.infinity { return self }
        if self == 0 { return Float.leastNonzeroMagnitude }
        return Float(bitPattern: self > 0 ? bitPattern + 1 : bitPattern - 1)
    }
    var nextDown: Float { -(-self).nextUp }
    var ulp: Float {
        if !isFinite { return Float.nan }
        let a = _fabs(self)
        if a == Float.greatestFiniteMagnitude { return a - a.nextDown }
        return a.nextUp - a
    }

    func truncatingRemainder(dividingBy other: Float) -> Float { _fmod(self, other) }
    mutating func formTruncatingRemainder(dividingBy other: Float) { self = _fmod(self, other) }

    // IEEE 754's remainder: self minus other times the nearest integer to
    // their quotient, ties to even. Exact, as every step below is.
    func remainder(dividingBy other: Float) -> Float {
        if isNaN || other.isNaN || isInfinite || other == 0 { return Float.nan }
        if other.isInfinite { return self }
        let m = _fabs(other)
        var a = _fabs(self)
        var odd = false
        if (m + m).isFinite { a = _fmod(a, m + m) }
        if a >= m {
            a = a - m
            odd = true
        }
        if a + a > m || (a + a == m && odd) { a = a - m }
        return isSignMinus ? -a : a
    }
    mutating func formRemainder(dividingBy other: Float) { self = remainder(dividingBy: other) }
}

// fmod: x less the whole multiples of y that fit, exactly. d runs down
// the powers of two times |y| no larger than |x|, and each subtraction is
// of numbers within a factor of two of each other, which is exact.
func _fmod(_ x: Float, _ y: Float) -> Float {
    if x.isNaN || y.isNaN || x.isInfinite || y == 0 { return Float.nan }
    if y.isInfinite { return x }
    var r = _fabs(x)
    let m = _fabs(y)
    var d = m
    while d + d <= r { d = d + d }
    while d >= m {
        if r >= d { r = r - d }
        d = d * 0.5
    }
    return _copysign(r, x)
}

func abs(_ x: Float) -> Float { _fabs(x) }

// The least and greatest of three or more.
func min<T: Comparable>(_ x: T, _ y: T, _ z: T, _ rest: T...) -> T {
    var m = min(min(x, y), z)
    for r in rest where r < m { m = r }
    return m
}
func max<T: Comparable>(_ x: T, _ y: T, _ z: T, _ rest: T...) -> T {
    var m = max(max(x, y), z)
    for r in rest where r > m { m = r }
    return m
}

// ---- Metatypes ----

// Two types are the same type where their metadata is the same record.
func == (t0: Any.Type?, t1: Any.Type?) -> Bool {
    if let a = t0 {
        if let b = t1 { return _same(a, b) }
        return false
    }
    if let _ = t1 { return false }
    return true
}

func != (t0: Any.Type?, t1: Any.Type?) -> Bool { !(t0 == t1) }

// ---- Failure ----

// Ends the program: the message goes to standard error and the process
// traps, as Swift's does.
func fatalError(_ message: String = "") -> Never { _fatalErrorMessage(message) }
func preconditionFailure(_ message: String = "") -> Never { _fatalErrorMessage(message) }

// Ends the program where condition is false.
func precondition(_ condition: Bool, _ message: String = "") {
    if !condition { _fatalErrorMessage(message) }
}
func assert(_ condition: Bool, _ message: String = "") {
    if !condition { _fatalErrorMessage("Assertion failed: " + message) }
}
func assertionFailure(_ message: String = "") { _fatalErrorMessage("Assertion failed: " + message) }

// ---- concurrency ----

// Where an AsyncStream's elements wait for its iterator: what the build
// closure yielded, in order, and whether it has finished.
final class _AsyncStreamStorage<Element> {
    var buffer: [Element]
    var finished: Bool

    init() {
        buffer = []
        finished = false
    }
}

// What an AsyncStream's build closure yields elements to, and finishes:
// AsyncStream<Element>.Continuation.
struct _AsyncStreamContinuation<Element> {
    let _storage: _AsyncStreamStorage<Element>

    func yield(_ value: Element) {
        _storage.buffer.append(value)
    }

    func finish() {
        _storage.finished = true
    }
}

// An AsyncStream's iterator: AsyncStream<Element>.AsyncIterator. It waits
// -- yielding the thread -- until the next element is there, or the
// stream has finished.
struct _AsyncStreamIterator<Element>: AsyncIteratorProtocol {
    let _storage: _AsyncStreamStorage<Element>
    var _next: Int

    mutating func next() async -> Element? {
        while _next >= _storage.buffer.count {
            if _storage.finished { return nil }
            await Task.yield()
        }
        let value = _storage.buffer[_next]
        _next += 1
        return value
    }
}

// A sequence of elements a closure yields over time, gone through with
// `for await`.
struct AsyncStream<Element>: AsyncSequence {
    let _storage: _AsyncStreamStorage<Element>

    init(_ build: (_AsyncStreamContinuation<Element>) -> Void) {
        _storage = _AsyncStreamStorage<Element>()
        build(_AsyncStreamContinuation<Element>(_storage: _storage))
    }

    init(_ elementType: Element.Type, _ build: (_AsyncStreamContinuation<Element>) -> Void) {
        _storage = _AsyncStreamStorage<Element>()
        build(_AsyncStreamContinuation<Element>(_storage: _storage))
    }

    func makeAsyncIterator() -> _AsyncStreamIterator<Element> {
        _AsyncStreamIterator<Element>(_storage: _storage, _next: 0)
    }
}

// A group of child tasks a withTaskGroup body adds, whose results it
// goes through as they are asked for -- here, in the order the tasks were
// added, which is one of the orders Swift may give them in.
struct TaskGroup<ChildTaskResult>: AsyncSequence, AsyncIteratorProtocol {
    var _tasks: [Task<ChildTaskResult, Never>]
    var _next: Int

    // Starts operation as a child task of the group.
    mutating func addTask(operation: @escaping () async -> ChildTaskResult) {
        _tasks.append(Task { await operation() })
    }

    // The next child's result, waited for; nil once there are none left.
    mutating func next() async -> ChildTaskResult? {
        if _next >= _tasks.count { return nil }
        let task = _tasks[_next]
        _next += 1
        return await task.value
    }

    // Waits for every child still running.
    mutating func waitForAll() async {
        while _next < _tasks.count {
            _ = await next()
        }
    }

    var isEmpty: Bool { _next >= _tasks.count }

    func makeAsyncIterator() -> TaskGroup<ChildTaskResult> { self }
}

// Runs body with a group it adds child tasks to, and waits for every one
// of them before returning what body returns.
func withTaskGroup<ChildTaskResult, GroupResult>(of childTaskResultType: ChildTaskResult.Type,
                                                 body: (inout TaskGroup<ChildTaskResult>) async -> GroupResult) async -> GroupResult {
    var group = TaskGroup<ChildTaskResult>(_tasks: [], _next: 0)
    let result = await body(&group)
    await group.waitForAll()
    return result
}

extension Task {
    // Whether the task this runs in has been cancelled.
    static var isCancelled: Bool { _vertexTaskIsCancelled() }

    // Asks the task to stop: it sees Task.isCancelled, and its sleeps
    // end at once.
    func cancel() { _vertexTaskCancel(handle) }
}

// ---- Float16 and BFloat16 ----
//
// Built in, as Float is (core.swift): each operation is one instruction
// of the machine's, where it has one. What a half float is beyond its
// arithmetic is written here the way Float's is.

extension Float16 {
    static var greatestFiniteMagnitude: Float16 { _float16(0x7BFF) }
    static var leastNormalMagnitude: Float16 { _float16(0x0400) }
    static var leastNonzeroMagnitude: Float16 { _float16(0x0001) }
    static var infinity: Float16 { _float16(0x7C00) }
    static var nan: Float16 { _float16(0x7E00) }
    static var ulpOfOne: Float16 { _float16(0x1400) }
    static var pi: Float16 { _float16(0x4248) }

    init(bitPattern bits: UInt16) { self = _float16(bits) }
    var bitPattern: UInt16 { _bits(self) }

    var isNaN: Bool { self != self }
    var isInfinite: Bool { bitPattern & 0x7FFF == 0x7C00 }
    var isFinite: Bool { bitPattern & 0x7c00 != 0x7c00 }
    var isZero: Bool { bitPattern & 0x7FFF == 0 }
    var isNormal: Bool {
        let e = bitPattern & 0x7c00
        return e != 0 && e != 0x7c00
    }
    var isSubnormal: Bool { bitPattern & 0x7c00 == 0 && bitPattern & 0x3ff != 0 }
    var magnitude: Float16 { _fabs(self) }
    var sign: FloatingPointSign { bitPattern >> 15 == 0 ? .plus : .minus }
    var isSignMinus: Bool { bitPattern >> 15 != 0 }

    func squareRoot() -> Float16 { _sqrt(self) }
    mutating func formSquareRoot() { self = _sqrt(self) }

    func rounded(_ rule: FloatingPointRoundingRule) -> Float16 {
        switch rule {
        case .toNearestOrAwayFromZero:
            let t = _trunc(self)
            if _fabs(self - t) >= 0.5 { return t + _copysign(1, self) }
            return t
        case .toNearestOrEven:
            return _rint(self)
        case .up:
            return _ceil(self)
        case .down:
            return _floor(self)
        case .towardZero:
            return _trunc(self)
        case .awayFromZero:
            let t = _trunc(self)
            return t == self ? self : t + _copysign(1, self)
        }
    }
    func rounded() -> Float16 { rounded(.toNearestOrAwayFromZero) }
    mutating func round() { self = rounded() }
    mutating func round(_ rule: FloatingPointRoundingRule) { self = rounded(rule) }

    var nextUp: Float16 {
        if isNaN || self == Float16.infinity { return self }
        if self == 0 { return Float16.leastNonzeroMagnitude }
        return Float16(bitPattern: self > 0 ? bitPattern + 1 : bitPattern - 1)
    }
    var nextDown: Float16 { -(-self).nextUp }
    var ulp: Float16 {
        if !isFinite { return Float16.nan }
        let a = _fabs(self)
        if a == Float16.greatestFiniteMagnitude { return a - a.nextDown }
        return a.nextUp - a
    }

}

extension Float16: CustomStringConvertible, CustomDebugStringConvertible {
    var description: String { _float16Description(UInt32(bitPattern)) }
    var debugDescription: String { _float16Description(UInt32(bitPattern)) }
}

func abs(_ x: Float16) -> Float16 { _fabs(x) }

extension BFloat16 {
    static var greatestFiniteMagnitude: BFloat16 { _bfloat16(0x7F7F) }
    static var leastNormalMagnitude: BFloat16 { _bfloat16(0x0080) }
    static var leastNonzeroMagnitude: BFloat16 { _bfloat16(0x0001) }
    static var infinity: BFloat16 { _bfloat16(0x7F80) }
    static var nan: BFloat16 { _bfloat16(0x7FC0) }
    static var ulpOfOne: BFloat16 { _bfloat16(0x3C00) }
    static var pi: BFloat16 { _bfloat16(0x4049) }

    init(bitPattern bits: UInt16) { self = _bfloat16(bits) }
    var bitPattern: UInt16 { _bits(self) }

    var isNaN: Bool { self != self }
    var isInfinite: Bool { bitPattern & 0x7FFF == 0x7F80 }
    var isFinite: Bool { bitPattern & 0x7f80 != 0x7f80 }
    var isZero: Bool { bitPattern & 0x7FFF == 0 }
    var isNormal: Bool {
        let e = bitPattern & 0x7f80
        return e != 0 && e != 0x7f80
    }
    var isSubnormal: Bool { bitPattern & 0x7f80 == 0 && bitPattern & 0x7f != 0 }
    var magnitude: BFloat16 { _fabs(self) }
    var sign: FloatingPointSign { bitPattern >> 15 == 0 ? .plus : .minus }
    var isSignMinus: Bool { bitPattern >> 15 != 0 }

    func squareRoot() -> BFloat16 { _sqrt(self) }
    mutating func formSquareRoot() { self = _sqrt(self) }

    func rounded(_ rule: FloatingPointRoundingRule) -> BFloat16 {
        switch rule {
        case .toNearestOrAwayFromZero:
            let t = _trunc(self)
            if _fabs(self - t) >= 0.5 { return t + _copysign(1, self) }
            return t
        case .toNearestOrEven:
            return _rint(self)
        case .up:
            return _ceil(self)
        case .down:
            return _floor(self)
        case .towardZero:
            return _trunc(self)
        case .awayFromZero:
            let t = _trunc(self)
            return t == self ? self : t + _copysign(1, self)
        }
    }
    func rounded() -> BFloat16 { rounded(.toNearestOrAwayFromZero) }
    mutating func round() { self = rounded() }
    mutating func round(_ rule: FloatingPointRoundingRule) { self = rounded(rule) }

    var nextUp: BFloat16 {
        if isNaN || self == BFloat16.infinity { return self }
        if self == 0 { return BFloat16.leastNonzeroMagnitude }
        return BFloat16(bitPattern: self > 0 ? bitPattern + 1 : bitPattern - 1)
    }
    var nextDown: BFloat16 { -(-self).nextUp }
    var ulp: BFloat16 {
        if !isFinite { return BFloat16.nan }
        let a = _fabs(self)
        if a == BFloat16.greatestFiniteMagnitude { return a - a.nextDown }
        return a.nextUp - a
    }

}

extension BFloat16: CustomStringConvertible, CustomDebugStringConvertible {
    var description: String { _bfloat16Description(UInt32(bitPattern)) }
    var debugDescription: String { _bfloat16Description(UInt32(bitPattern)) }
}

func abs(_ x: BFloat16) -> BFloat16 { _fabs(x) }

// ---- OptionSet ----

extension OptionSet {
    // The flags of every element, together: what an array literal written
    // where an option set goes makes, as Swift's init(arrayLiteral:) does.
    static func _union(_ elements: [Self]) -> Self {
        var bits = RawValue(0)
        for e in elements { bits = bits | e.rawValue }
        return Self(rawValue: bits)
    }

    var isEmpty: Bool { return rawValue == RawValue(0) }

    // Whether every flag of member is set.
    func contains(_ member: Self) -> Bool { return rawValue & member.rawValue == member.rawValue }

    func union(_ other: Self) -> Self { return Self(rawValue: rawValue | other.rawValue) }
    func intersection(_ other: Self) -> Self { return Self(rawValue: rawValue & other.rawValue) }
    func symmetricDifference(_ other: Self) -> Self { return Self(rawValue: rawValue ^ other.rawValue) }
    func subtracting(_ other: Self) -> Self { return Self(rawValue: rawValue & (rawValue ^ other.rawValue)) }

    func isSubset(of other: Self) -> Bool { return rawValue & other.rawValue == rawValue }
    func isSuperset(of other: Self) -> Bool { return rawValue & other.rawValue == other.rawValue }
    func isDisjoint(with other: Self) -> Bool { return (rawValue & other.rawValue) == RawValue(0) }

    mutating func formUnion(_ other: Self) { self = union(other) }
    mutating func formIntersection(_ other: Self) { self = intersection(other) }
    mutating func formSymmetricDifference(_ other: Self) { self = symmetricDifference(other) }
    mutating func subtract(_ other: Self) { self = subtracting(other) }

    // Sets newMember's flags; whether any was not set already, and them.
    @discardableResult
    mutating func insert(_ newMember: Self) -> (inserted: Bool, memberAfterInsert: Self) {
        let had = contains(newMember)
        self = union(newMember)
        return (!had, newMember)
    }

    // Clears member's flags; the ones that were set, or nil where none was.
    @discardableResult
    mutating func remove(_ member: Self) -> Self? {
        let had = intersection(member)
        self = subtracting(member)
        return had.isEmpty ? nil : had
    }

    // Sets newMember's flags; what was set of them before, or nil.
    @discardableResult
    mutating func update(with newMember: Self) -> Self? {
        let had = intersection(newMember)
        self = union(newMember)
        return had.isEmpty ? nil : had
    }
}

// ---- Sequence ----

// What every sequence can do by going through it: Swift's Sequence
// algorithms, each answering an array where Swift's does. A type's own
// member of the name -- Array's map -- is the one its values use.
extension Sequence {
    // A new array of what transform makes of each element, in order.
    func map<T>(_ transform: (Element) throws -> T) rethrows -> [T] {
        var out: [T] = []
        var it = makeIterator()
        while let x = it.next() { out.append(try transform(x)) }
        return out
    }

    // The elements isIncluded keeps, in order.
    func filter(_ isIncluded: (Element) throws -> Bool) rethrows -> [Element] {
        var out: [Element] = []
        var it = makeIterator()
        while let x = it.next() {
            if try isIncluded(x) { out.append(x) }
        }
        return out
    }

    // The elements combined in order, starting from initialResult.
    func reduce<Result>(_ initialResult: Result, _ nextPartialResult: (Result, Element) throws -> Result) rethrows -> Result {
        var acc = initialResult
        var it = makeIterator()
        while let x = it.next() { acc = try nextPartialResult(acc, x) }
        return acc
    }

    // The elements combined in order into initialResult, changed in place.
    func reduce<Result>(into initialResult: Result, _ updateAccumulatingResult: (inout Result, Element) throws -> Void) rethrows -> Result {
        var acc = initialResult
        var it = makeIterator()
        while let x = it.next() { try updateAccumulatingResult(&acc, x) }
        return acc
    }

    // body, called with each element in order.
    func forEach(_ body: (Element) throws -> Void) rethrows {
        var it = makeIterator()
        while let x = it.next() { try body(x) }
    }

    // The non-nil results of transform, in order.
    func compactMap<T>(_ transform: (Element) throws -> T?) rethrows -> [T] {
        var out: [T] = []
        var it = makeIterator()
        while let x = it.next() {
            if let y = try transform(x) { out.append(y) }
        }
        return out
    }

    // The first element predicate is true of, if any.
    func first(where predicate: (Element) throws -> Bool) rethrows -> Element? {
        var it = makeIterator()
        while let x = it.next() {
            if try predicate(x) { return x }
        }
        return nil
    }

    // Whether predicate is true of some element.
    func contains(where predicate: (Element) throws -> Bool) rethrows -> Bool {
        var it = makeIterator()
        while let x = it.next() {
            if try predicate(x) { return true }
        }
        return false
    }

    // Whether predicate is true of every element.
    func allSatisfy(_ predicate: (Element) throws -> Bool) rethrows -> Bool {
        var it = makeIterator()
        while let x = it.next() {
            if !(try predicate(x)) { return false }
        }
        return true
    }

    // The elements, in order, as an array.
    var _array: [Element] {
        var out: [Element] = []
        var it = makeIterator()
        while let x = it.next() { out.append(x) }
        return out
    }

    // The elements in the order areInIncreasingOrder puts them.
    func sorted(by areInIncreasingOrder: (Element, Element) -> Bool) -> [Element] {
        return _array.sorted(by: areInIncreasingOrder)
    }

    // The elements, last first.
    func reversed() -> [Element] { return _array.reversed() }

    // Each element with its position, counting from 0.
    func enumerated() -> [(offset: Int, element: Element)] {
        var out: [(offset: Int, element: Element)] = []
        var n = 0
        var it = makeIterator()
        while let x = it.next() {
            out.append((offset: n, element: x))
            n += 1
        }
        return out
    }
}

// An iterator that hands out what a function answers, until it answers nil;
// a sequence of those elements too.
struct AnyIterator<Element>: IteratorProtocol, Sequence {
    let _body: () -> Element?

    init(_ body: @escaping () -> Element?) { _body = body }

    mutating func next() -> Element? { return _body() }
}

// A sequence that is its own iterator goes through itself, as Swift's
// `extension Sequence where Self.Iterator == Self` has it.
extension Sequence where Self: IteratorProtocol {
    func makeIterator() -> Self { return self }
}

// ---- Collection ----

// What goes through a collection by its positions: a collection's
// iterator, where it declares none of its own.
struct IndexingIterator<Elements: Collection>: IteratorProtocol {
    let _elements: Elements
    var _position: Elements.Index

    mutating func next() -> Elements.Element? {
        if _position == _elements.endIndex { return nil }
        let x = _elements[_position]
        _position = _elements.index(after: _position)
        return x
    }
}

extension Collection {
    func makeIterator() -> IndexingIterator<Self> {
        return IndexingIterator(_elements: self, _position: startIndex)
    }

    var isEmpty: Bool { return startIndex == endIndex }

    // How many elements there are.
    var count: Int {
        var n = 0
        var i = startIndex
        while i != endIndex {
            n += 1
            i = index(after: i)
        }
        return n
    }

    // The first element, or nil where there is none.
    var first: Element? { return isEmpty ? nil : self[startIndex] }

    // All but the first k elements.
    func dropFirst(_ k: Int = 1) -> [Element] {
        var out: [Element] = []
        var i = startIndex
        var n = 0
        while i != endIndex {
            if n >= k { out.append(self[i]) }
            n += 1
            i = index(after: i)
        }
        return out
    }

    // The first maxLength elements.
    func prefix(_ maxLength: Int) -> [Element] {
        var out: [Element] = []
        var i = startIndex
        while i != endIndex && out.count < maxLength {
            out.append(self[i])
            i = index(after: i)
        }
        return out
    }

    // Where the first element predicate is true of is, if anywhere.
    func firstIndex(where predicate: (Element) throws -> Bool) rethrows -> Index? {
        var i = startIndex
        while i != endIndex {
            if try predicate(self[i]) { return i }
            i = index(after: i)
        }
        return nil
    }

    // The position distance steps after i.
    func index(_ i: Index, offsetBy distance: Int) -> Index {
        var at = i
        var n = 0
        while n < distance {
            at = index(after: at)
            n += 1
        }
        return at
    }

    // How many steps it is from start to end.
    func distance(from start: Index, to end: Index) -> Int {
        var n = 0
        var i = start
        while i != end {
            n += 1
            i = index(after: i)
        }
        return n
    }
}

extension Collection where Element: Equatable {
    // Where the first element equal to element is, if anywhere.
    func firstIndex(of element: Element) -> Index? {
        var i = startIndex
        while i != endIndex {
            if self[i] == element { return i }
            i = index(after: i)
        }
        return nil
    }

    // Whether an element equals element.
    func contains(_ element: Element) -> Bool { return firstIndex(of: element) != nil }
}

extension BidirectionalCollection {
    // The last element, or nil where there is none.
    var last: Element? { return isEmpty ? nil : self[index(before: endIndex)] }
}

// Integer positions step by one, as Swift's Strideable indices do.
extension Collection where Index == Int {
    func index(after i: Int) -> Int { return i + 1 }
}

extension BidirectionalCollection where Index == Int {
    func index(before i: Int) -> Int { return i - 1 }
}

// An array is a collection of its elements at the positions 0 up to its
// count, gone through by position, as Swift's Array is.
extension Array: RandomAccessCollection {
    typealias Index = Int
    typealias Iterator = IndexingIterator<[Element]>

    var startIndex: Int { return 0 }
    var endIndex: Int { return count }
    func index(after i: Int) -> Int { return i + 1 }
    func index(before i: Int) -> Int { return i - 1 }

    func makeIterator() -> IndexingIterator<[Element]> {
        return IndexingIterator(_elements: self, _position: 0)
    }
}

// A range of integers is a collection of them, each at its own position,
// as Swift's ranges of Strideable integer bounds are.
extension Range: RandomAccessCollection where Bound == Int {
    var startIndex: Int { return lowerBound }
    var endIndex: Int { return upperBound }
    subscript(position: Int) -> Int { return position }
    func index(after i: Int) -> Int { return i + 1 }
    func index(before i: Int) -> Int { return i - 1 }
}

extension ClosedRange: RandomAccessCollection where Bound == Int {
    var startIndex: Int { return lowerBound }
    var endIndex: Int { return upperBound + 1 }
    subscript(position: Int) -> Int { return position }
    func index(after i: Int) -> Int { return i + 1 }
    func index(before i: Int) -> Int { return i - 1 }
}

// A string is a collection of its Characters, at the positions of their
// first bytes, as Swift's String is.
extension String: BidirectionalCollection {
    typealias Element = Character
    typealias Index = _StringIndex
    typealias Iterator = _StringIterator
}

extension _SetIndex: Equatable, Comparable {
    static func == (lhs: _SetIndex, rhs: _SetIndex) -> Bool { return lhs._bucket == rhs._bucket }
    static func < (lhs: _SetIndex, rhs: _SetIndex) -> Bool { return lhs._bucket < rhs._bucket }
}

// A set is a collection of its members, in bucket order.
extension Set: Collection {
    typealias Index = _SetIndex
    typealias Iterator = IndexingIterator<Set<Element>>

    func _index(_ bucket: Int) -> _SetIndex {
        return _SetIndex(_bucket: bucket < 0 ? Int.max : bucket)
    }

    var startIndex: _SetIndex { return _index(_bucket(after: 0)) }
    var endIndex: _SetIndex { return _SetIndex(_bucket: Int.max) }

    func index(after i: _SetIndex) -> _SetIndex {
        if i._bucket == Int.max { fatalError("Set index is out of bounds") }
        return _index(_bucket(after: i._bucket + 1))
    }

    subscript(position: _SetIndex) -> Element {
        if position._bucket == Int.max { fatalError("Set index is out of bounds") }
        return _member(at: position._bucket)
    }
}

// A slice is a collection of its elements at its base array's positions,
// as Swift's ArraySlice is.
extension ArraySlice: RandomAccessCollection {
    typealias Index = Int
    typealias Iterator = _ArraySliceIterator<Element>

    func index(after i: Int) -> Int { return i + 1 }
    func index(before i: Int) -> Int { return i - 1 }
}

// Buffer pointers are collections of what they point at, at positions 0
// up to their count, as Swift's are.
extension UnsafeBufferPointer: RandomAccessCollection {
    typealias Index = Int
    typealias Iterator = _UnsafeBufferIterator<Element>

    func makeIterator() -> _UnsafeBufferIterator<Element> {
        return _UnsafeBufferIterator(_buffer: self, _at: 0)
    }

    var startIndex: Int { return 0 }
    var endIndex: Int { return count }
    func index(after i: Int) -> Int { return i + 1 }
    func index(before i: Int) -> Int { return i - 1 }

    subscript(i: Int) -> Element {
        return (baseAddress! + i).pointee
    }
}

extension UnsafeMutableBufferPointer: RandomAccessCollection {
    typealias Index = Int
    typealias Iterator = _UnsafeMutableBufferIterator<Element>

    func makeIterator() -> _UnsafeMutableBufferIterator<Element> {
        return _UnsafeMutableBufferIterator(_buffer: self, _at: 0)
    }

    var startIndex: Int { return 0 }
    var endIndex: Int { return count }
    func index(after i: Int) -> Int { return i + 1 }
    func index(before i: Int) -> Int { return i - 1 }

    subscript(i: Int) -> Element {
        get { return (baseAddress! + i).pointee }
        nonmutating set { (baseAddress! + i).pointee = newValue }
    }
}

// What for-in over a buffer pointer walks: its elements, in order.
struct _UnsafeBufferIterator<Element>: IteratorProtocol {
    let _buffer: UnsafeBufferPointer<Element>
    var _at: Int

    mutating func next() -> Element? {
        if _at >= _buffer.count { return nil }
        let x = (_buffer.baseAddress! + _at).pointee
        _at += 1
        return x
    }
}

struct _UnsafeMutableBufferIterator<Element>: IteratorProtocol {
    let _buffer: UnsafeMutableBufferPointer<Element>
    var _at: Int

    mutating func next() -> Element? {
        if _at >= _buffer.count { return nil }
        let x = (_buffer.baseAddress! + _at).pointee
        _at += 1
        return x
    }
}
