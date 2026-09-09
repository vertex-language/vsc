// §6.8 Subscripts with Custom Getters, Setters, and Generic Keys

struct Matrix {
    let rows: Int, columns: Int
    var grid: [Double]

    init(rows: Int, columns: Int) {
        self.rows = rows
        self.columns = columns
        grid = Array(repeating: 0.0, count: rows * columns)
    }

    subscript(row: Int, column: Int) -> Double {
        get {
            return grid[(row * columns) + column]
        }
        set {
            grid[(row * columns) + column] = newValue
        }
    }
}

struct TypedDictionary {
    private var storage = [String: Any]()

    subscript<T>(key: String, as type: T.Type, default defaultValue: @autoclosure () -> T) -> T {
        get {
            return (storage[key] as? T) ?? defaultValue()
        }
        set {
            storage[key] = newValue
        }
    }
}

enum Environment {
    private static var values = [String: String]()

    static subscript(key: String) -> String? {
        get { values[key] }
        set { values[key] = newValue }
    }
}
