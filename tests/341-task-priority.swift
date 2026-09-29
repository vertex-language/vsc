// Task(priority:): a task made with a priority runs and returns its value.
let t = Task(priority: .high) { 21 * 2 }
let u = Task(priority: .low) { "low" }
print(await t.value, await u.value)
