// §5.4 Nested Do-Catch, Compound Catch Clauses, and Pattern Guards

enum NetworkError: Error {
    case timeout
    case badStatus(code: Int, message: String)
    case unauthorized
}

enum DatabaseError: Error {
    case connectionLost
    case rowNotFound(id: Int)
}

func queryRemote(id: Int) throws -> String {
    if id < 0 { throw NetworkError.badStatus(code: 400, message: "Bad Request") }
    if id == 0 { throw NetworkError.unauthorized }
    return "Data for \(id)"
}

func executeWithFallback(id: Int) -> String {
    do {
        let result = try queryRemote(id: id)
        return result
    } catch NetworkError.timeout, NetworkError.unauthorized {
        return "Network retry needed"
    } catch NetworkError.badStatus(let code, let msg) where code >= 500 {
        return "Server error: \(code) - \(msg)"
    } catch NetworkError.badStatus(let code, _) where code == 404 {
        return "Not found"
    } catch let err as NetworkError {
        return "Other network error: \(err)"
    } catch {
        do {
            let fallbackResult = try queryRemote(id: 1)
            return "Nested fallback: \(fallbackResult)"
        } catch {
            return "Total failure"
        }
    }
}
