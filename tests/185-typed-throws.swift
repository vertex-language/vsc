// throws(E) states the error type, so catch binds an E without casting.
enum NetError: Error { case timeout(seconds: Int), refused }
func fetch(_ n: Int) throws(NetError) -> String {
    switch n {
    case 0: throw .refused
    case 1...5: throw .timeout(seconds: n * 10)
    default: return "data \(n)"
    }
}
for n in [0, 3, 9] {
    do {
        print(try fetch(n))
    } catch {
        switch error {
        case .refused: print("refused")
        case .timeout(let s): print("timeout", s)
        }
    }
}
