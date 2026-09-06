// §8 Deep Nested Enum, Optional, and Tuple Pattern Matching

enum APIResult<T, E: Error> {
    case success(T)
    case failure(E)
    case pending(progress: Double)
}

enum ServicePayload {
    case data(id: Int, bytes: [UInt8]?)
    case message(String)
}

struct APIError: Error {
    let status: Int
    let detail: String
}

func parseAPIResult(result: APIResult<ServicePayload?, APIError>) -> String {
    switch result {
    case .success(.some(.data(id: let id, bytes: .some(let b)))) where !b.isEmpty:
        return "Received \(b.count) bytes for id \(id)"
    case .success(.some(.data(id: let id, bytes: _))):
        return "Empty data for id \(id)"
    case .success(.some(.message(let msg))):
        return "Message: \(msg)"
    case .success(.none):
        return "Null payload"
    case .failure(let err) where err.status >= 500:
        return "Server failure: \(err.detail)"
    case .failure(let err):
        return "Client error (\(err.status)): \(err.detail)"
    case .pending(progress: 0.0..<0.5):
        return "Starting..."
    case .pending(progress: 0.5...1.0):
        return "Almost done"
    default:
        return "Unknown state"
    }
}
