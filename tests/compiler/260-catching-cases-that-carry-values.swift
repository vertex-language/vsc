// Catching one case of an error enum whose cases carry values that own
// something -- a String, here -- and binding or ignoring what they carry.
//
// The error is boxed, and a clause that names a case tests a copy of it:
// on a match what the case carries leaves the copy for the clause's names,
// and on a miss the copy is let go and the box goes on to the next clause
// untouched. Every case carrying a String is the shape nearly every error
// enum a library declares has, so this is the ordinary catch.
enum NetError: Error {
    case timedOut(String)
    case refused(String)
    case limit(limit: Int, context: String)
    case plain
}

func fail(_ n: Int) throws {
    switch n {
    case 0: throw NetError.timedOut("read")
    case 1: throw NetError.refused("10.0.0.1")
    case 2: throw NetError.limit(limit: 5, context: "body")
    case 3: throw NetError.plain
    default: return
    }
}

func classify(_ n: Int) -> Int {
    do {
        try fail(n)
        return 0
    } catch NetError.timedOut(let what) {
        return 10 + what.count
    } catch NetError.refused(_) {
        return 20
    } catch NetError.limit(let limit, _) {
        return 30 + limit
    } catch NetError.plain {
        return 40
    } catch {
        return 99
    }
}

func main() -> Int32 {
    var total = 0
    // Many times over, so that a copy leaked or released twice on some path
    // shows up as a crash rather than hiding in one pass.
    for _ in 0..<2000 {
        total += classify(0) + classify(1) + classify(2) + classify(3) + classify(4)
    }
    return Int32(total % 251)
}
