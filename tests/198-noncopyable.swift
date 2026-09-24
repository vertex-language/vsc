// A ~Copyable type is moved, never copied; a consuming method ends its life.
struct Ticket: ~Copyable {
    let id: Int
    init(_ id: Int) {
        self.id = id
        print("issue", id)
    }
    borrowing func peek() -> Int { id }
    consuming func redeem() -> Int {
        print("redeem", id)
        return id * 10
    }
    deinit { print("deinit", id) }
}
func inspect(_ t: borrowing Ticket) -> Int { t.peek() + 1 }
func run() {
    let a = Ticket(1)
    print(a.peek(), inspect(a))
    print(a.redeem())
    let b = Ticket(2)
    let moved = consume b
    print("moved", moved.id)
}
run()
print("end")
