// A closure that uses a variable shares it with the scope it was declared
// in: writes on either side are seen on the other, a counter a function
// returns keeps a variable of its own, and two closures over one variable
// change the same storage.
func makeCounter() -> () -> Int {
    var count = 0
    return {
        count += 1
        return count
    }
}

func run(_ f: () -> Void) {
    f()
}

func main() -> Int32 {
    var total = 0
    run { total += 5 }
    run { total += 10 }

    var log = "start"
    let append = { (s: String) in
        log = log + " " + s
    }
    append("a")
    append("b")
    if log == "start a b" {
        total += 100
    }

    let next = makeCounter()
    let other = makeCounter()
    total += next() + next() + next() + other()

    var shared = 1
    let double = { shared *= 2 }
    let addOne = { shared += 1 }
    double()
    addOne()
    double()
    total += shared * 10
    shared = 100
    addOne()
    total += shared

    return Int32(total % 251)
}
