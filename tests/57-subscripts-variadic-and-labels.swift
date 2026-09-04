// §6.8 Subscripts with Variadics, Argument Labels, and Defaults

struct Tensor {
    private var data: [Double] = []

    subscript(indices: Int...) -> Double {
        get {
            let offset = indices.reduce(0, +)
            return data.indices.contains(offset) ? data[offset] : 0.0
        }
        set {
            let offset = indices.reduce(0, +)
            if data.indices.contains(offset) {
                data[offset] = newValue
            }
        }
    }

    subscript(at position: Int, mode mode: String = "strict") -> Double {
        get { data[position] }
        set { data[position] = newValue }
    }
}

enum Registry {
    private static var map: [String: Any] = [:]

    static subscript<T>(key: String, type type: T.Type, default fallback: @autoclosure () -> T) -> T {
        get {
            return (map[key] as? T) ?? fallback()
        }
        set {
            map[key] = newValue
        }
    }
}

func testSubscriptFeatures() {
    var tensor = Tensor()
    tensor[0, 1, 2] = 4.2
    _ = tensor[0, 1, 2]
    _ = tensor[at: 0]
    _ = tensor[at: 0, mode: "lenient"]
    Registry["port", type: Int.self, default: 8080] = 9090
}
