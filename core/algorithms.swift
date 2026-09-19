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

    // What transform makes of each element, with nothing where it makes nil.
    func compactMap<T>(_ transform: (Element) -> T?) -> [T] {
        var out: [T] = []
        for x in self {
            if let y = transform(x) { out.append(y) }
        }
        return out
    }

    // The arrays transform makes of each element, one after another.
    func flatMap<T>(_ transform: (Element) -> [T]) -> [T] {
        var out: [T] = []
        for x in self { out.append(contentsOf: transform(x)) }
        return out
    }

    // body, called with each element in order.
    func forEach(_ body: (Element) -> Void) {
        for x in self { body(x) }
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

extension Array where Element: Equatable {
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
