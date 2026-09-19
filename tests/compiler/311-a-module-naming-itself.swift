// A module may name itself to reach its own declarations: `main.f()`,
// `main.T`, `main.limit`, even beside a function named main.

struct ConnectionState { var open = true }
func connectionState() -> ConnectionState { return main.ConnectionState() }
enum Mode { case fast, slow }
let limit = 3

func main() -> Int32 {
    let s = main.ConnectionState(open: false)
    let t = main.connectionState()
    let m: main.Mode = .fast
    print(s.open, t.open, m == main.Mode.fast, main.limit)
    return Int32(main.limit)
}
