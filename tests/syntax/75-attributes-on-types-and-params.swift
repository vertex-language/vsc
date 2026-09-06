// §7 / §3.3 Attributes on Types, Parameters, and Function Signatures

typealias CCallback = @convention(c) (Int32, Int32) -> Int32
typealias BlockCallback = @convention(block) (String) -> Void
typealias AsyncSendableTransform = @Sendable (Int) async throws -> String

func processAsyncHandler(
    condition: @autoclosure @escaping () -> Bool,
    handler: @escaping @Sendable () async -> Void
) {
    Task {
        if condition() {
            await handler()
        }
    }
}

struct CallbackRegistry {
    var cFunc: CCallback
    var blockFunc: BlockCallback
    var tupleOfClosures: (
        @Sendable () -> Void,
        @Sendable (Int) -> Int
    )
}

func testAttributeSignatures() {
    let add: CCallback = { a, b in a + b }
    let printBlock: BlockCallback = { str in print(str) }
    let registry = CallbackRegistry(
        cFunc: add,
        blockFunc: printBlock,
        tupleOfClosures: (
            { print("void") },
            { $0 * 2 }
        )
    )
    _ = registry
}
