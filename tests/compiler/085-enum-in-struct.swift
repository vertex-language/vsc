// An enum stored as a field.
enum State {
    case idle
    case running
    case done
}

struct Task {
    var id: Int32
    var state: State
}

func score(_ t: Task) -> Int32 {
    switch t.state {
    case .idle: return t.id
    case .running: return t.id * 10
    case .done: return t.id * 100
    }
}

func main() -> Int32 {
    return score(Task(id: 1, state: .done)) - score(Task(id: 2, state: .running)) -
           score(Task(id: 8, state: .idle)) * 4
}
