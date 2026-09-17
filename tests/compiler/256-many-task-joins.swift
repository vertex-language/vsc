// Waiting on a task that has already finished. Nothing has to be waited
// for, so the task carries on -- but it carries on from the executor
// rather than from inside whatever answered it. A runtime that ran the
// rest of the task nested under that call would put a frame on the stack
// for every one of these, and this loop would exhaust it.
//
// The tasks share nothing, since Swift runs them on threads of its own;
// main counts only what it waited for.
func tick() async -> Int {
    return 1
}

func main() async -> Int32 {
    let t = Task { await tick() }
    var total = 0
    for _ in 0..<200_000 {
        total += await t.value
    }
    return Int32(total % 251)
}
