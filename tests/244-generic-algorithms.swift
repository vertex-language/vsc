// Classic algorithms written once over generic collections.
func binarySearch<C: RandomAccessCollection>(_ xs: C, _ x: C.Element) -> C.Index? where C.Element: Comparable {
    var lo = xs.startIndex, hi = xs.endIndex
    while lo < hi {
        let mid = xs.index(lo, offsetBy: xs.distance(from: lo, to: hi) / 2)
        if xs[mid] == x { return mid }
        if xs[mid] < x { lo = xs.index(after: mid) } else { hi = mid }
    }
    return nil
}
func insertionSort<T: Comparable>(_ xs: [T]) -> [T] {
    var a = xs
    for i in a.indices.dropFirst() {
        var j = i
        while j > 0 && a[j - 1] > a[j] { a.swapAt(j - 1, j); j -= 1 }
    }
    return a
}
let data = insertionSort([9, 4, 7, 1, 8, 2])
print(data, binarySearch(data, 7) as Any, binarySearch(data, 5) as Any)
print(insertionSort(["pear", "fig", "apple"]), binarySearch("aceg".map { $0 }, "e") as Any)
