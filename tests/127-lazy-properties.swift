// A lazy property is computed on first use, once.
struct Report {
    var rows: [Int]
    lazy var total: Int = {
        print("computing total")
        return rows.reduce(0, +)
    }()
}
var r = Report(rows: [1, 2, 3])
print("made")
print(r.total)
print(r.total)
