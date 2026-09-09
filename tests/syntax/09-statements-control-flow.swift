// §5 Statements

// ForInStatement with await/case/WhereClause
func forInExamples() {
    for number in 1...5 where number % 2 == 0 {
        print(number)
    }

    let optionalItems: [Int?] = [1, nil, 3]
    for case let value? in optionalItems {
        print(value)
    }
}

// WhileStatement with ConditionList
func whileExample() {
    var i = 0
    while i < 5, i != 3 {
        i += 1
    }
}

// RepeatWhileStatement
func repeatExample() {
    var i = 0
    repeat {
        i += 1
    } while i < 3
}

// IfStatement / ElseClause / GuardStatement
func branchExamples(_ value: Int?) {
    if let unwrapped = value, unwrapped > 0 {
        print("positive: \(unwrapped)")
    } else if value == 0 {
        print("zero")
    } else {
        print("negative or nil")
    }

    guard let unwrapped = value else {
        print("no value")
        return
    }
    print(unwrapped)
}

// SwitchStatement with CaseLabel / DefaultLabel / WhereClause
func switchExample(_ value: Int) {
    switch value {
    case 0:
        print("zero")
    case let n where n < 0:
        print("negative \(n)")
    case 1, 2, 3:
        print("small")
    default:
        print("other")
    }
}

// LabeledStatement / ControlTransferStatement
func labeledLoopExample() {
    outer: for i in 0..<3 {
        for j in 0..<3 {
            if j == 1 { continue outer }
            if i == 2 { break outer }
            print(i, j)
        }
    }
}

// FallthroughStatement
func fallthroughExample(_ value: Int) {
    switch value {
    case 1:
        print("one")
        fallthrough
    case 2:
        print("one or two")
    default:
        print("other")
    }
}

// ThrowStatement / DoStatement / CatchClause
enum FileError: Error { case notFound }
func readFile() throws -> String { throw FileError.notFound }

func doCatchExample() {
    do {
        let contents = try readFile()
        print(contents)
    } catch FileError.notFound {
        print("not found")
    } catch {
        print("other error: \(error)")
    }
}

// DeferStatement
func deferExample() {
    defer { print("cleanup") }
    print("work")
}

// DiscardStatement (noncopyable types)
struct Resource: ~Copyable {
    consuming func release() {
        discard self
    }
}