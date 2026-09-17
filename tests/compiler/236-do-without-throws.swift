// A do whose body throws nothing still runs its body; its catch clauses
// are never reached, which Swift warns about and runs nothing of.
struct Bad: Error {}

func risky(_ n: Int) throws -> Int {
    if n < 0 {
        throw Bad()
    }
    return n * 2
}

func quiet(_ n: Int) -> Int {
    var total = 0
    do {
        total = n + 1
    } catch {
        total = -1
    }
    return total
}

func early(_ n: Int) -> Int {
    do {
        return n * 3
    } catch {
        return 0
    }
}

func main() -> Int32 {
    var total = quiet(4) + early(5)
    do {
        let name: String
        name = "reached"
        total += name.count
    } catch {
        total += 1000
    }
    do {
        do {
            total += 2
        } catch {
            total += 500
        }
        total += try risky(-1)
    } catch {
        total += 7
    }
    return Int32(total % 251)
}
