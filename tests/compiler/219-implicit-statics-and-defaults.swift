// A static property whose type is the one a context wants is named the
// way a case is: `.default` where an Options goes. A default argument
// that is not a constant is evaluated at each call that leaves it out.
struct Options {
    var backlog: Int = 128
    var reuse: Bool = false

    static let `default` = Options()
    static var fast: Options {
        return Options(backlog: 1024, reuse: true)
    }
}

func listen(port: Int, options: Options = .default) -> Int {
    return port + options.backlog + (options.reuse ? 1 : 0)
}


func main() -> Int32 {
    var total = listen(port: 1)
    total += listen(port: 2, options: .fast)
    let chosen: Options = .fast
    total += chosen.backlog
    return Int32(total % 251)
}
