// Ultimate Syntactic and Grammar Stress Integration (Volume 2)

import Foundation

// 1. Right-Associative Pipeline Operator
precedencegroup StepPipelinePrecedence {
    associativity: right
    higherThan: TernaryPrecedence
}

infix operator ~>>>: StepPipelinePrecedence

func ~>>> <In, Out>(val: In, transform: (In) throws -> Out) rethrows -> Out {
    return try transform(val)
}

// 2. Noncopyable Channel with Typed Throws
enum ChannelError: Error {
    case channelClosed
    case capacityExceeded(Int)
}

struct UnbufferedChannel<T>: ~Copyable {
    private var buffer: [T] = []

    init() {}

    deinit {
        // cleanup channel
    }

    borrowing func count() -> Int {
        return buffer.count
    }

    mutating func send(_ item: T) throws(ChannelError) {
        if buffer.count >= 10 {
            throw .capacityExceeded(buffer.count)
        }
        buffer.append(item)
    }

    consuming func close() {
        discard self
    }
}

// 3. Multi-Pack Generic Function with Trailing Comma
func zipPacksWithTransform<each T, each U, Result>(
    first: repeat each T,
    second: repeat each U,
    transform: (repeat each T, repeat each U,) -> Result,
) -> Result {
    return transform(repeat each first, repeat each second,)
}

// 4. Result Builder
@resultBuilder
struct CommandBuilder {
    static func buildBlock(_ commands: String...) -> [String] {
        return commands
    }
}

func buildExecutionPlan(@CommandBuilder plan: () -> [String]) -> [String] {
    return plan()
}

// 5. Integration Function Stressing All Language Corners
func runVolume2TortureTest() async throws {
    var channel = UnbufferedChannel<Int>()
    try channel.send(100)
    let cnt = channel.count()
    channel.close()
    _ = cnt

    let plan = buildExecutionPlan {
        "INIT"
        "RUN"
        "TEARDOWN"
    }
    _ = plan

    let combinedResult = zipPacksWithTransform(
        first: 1,
        "alpha",
        second: 2.0,
        true,
    ) { (a: Int, b: String, c: Double, d: Bool,) -> String in
        return "\(a)-\(b)-\(c)-\(d)"
    }
    print(combinedResult)

    let piped = try 10 ~>>> { x in "\(x * 10)" }
    _ = piped
}
