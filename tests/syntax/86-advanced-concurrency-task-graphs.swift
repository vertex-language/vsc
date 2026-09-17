// §6 Concurrency Task Graphs, Cancellations, and Dynamic Task Groups

actor CacheWorker {
    private var entries: [String: String] = [:]

    func get(_ key: String) -> String? {
        entries[key]
    }

    func set(_ key: String, _ value: String) {
        entries[key] = value
    }
}

func fetchConcurrently(keys: [String], worker: CacheWorker) async throws -> [String] {
    return try await withThrowingTaskGroup(of: String.self) { group in
        for key in keys {
            group.addTask {
                if let cached = await worker.get(key) {
                    return cached
                }
                return "Fetched: \(key)"
            }
        }

        var results: [String] = []
        for try await item in group {
            results.append(item)
        }
        return results
    }
}

func cancellableJob(operation: @Sendable @escaping () async -> Void) async {
    await withTaskCancellationHandler {
        await operation()
    } onCancel: {
        print("Job cancelled")
    }
}
