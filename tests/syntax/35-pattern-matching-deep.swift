// §8 Deep Pattern Matching

enum NetworkResponse {
    case success(statusCode: Int, payload: (headers: [String: String], body: String))
    case error(code: Int, message: String)
    case cancelled
}

struct Point3D {
    var x: Double
    var y: Double
    var z: Double
}

func ~= (pattern: ClosedRange<Double>, value: Point3D) -> Bool {
    let dist = (value.x * value.x + value.y * value.y + value.z * value.z).squareRoot()
    return pattern.contains(dist)
}

func matchComplex(response: NetworkResponse, pt: Point3D) {
    switch response {
    case .success(200..<300, (let headers, let body)) where !headers.isEmpty:
        print("OK body length: \(body.count)")
    case .success(let code, _) where code >= 400:
        print("HTTP error: \(code)")
    case .error(code: 404, message: let msg):
        print("Not found: \(msg)")
    case .error(code: let c, message: let m):
        print("Error \(c): \(m)")
    case .cancelled:
        print("Cancelled")
    default:
        break
    }

    switch pt {
    case 0.0...1.0:
        print("Near origin")
    case 1.0...10.0:
        print("Intermediate")
    default:
        print("Far away")
    }

    let optionalPairs: [(Int, Int)?] = [(1, 2), nil, (3, 4)]
    for case let (x, y)? in optionalPairs where x < y {
        print("Pair: \(x), \(y)")
    }
}
