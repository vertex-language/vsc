# The Vertex runtime ABI

What compiled Vertex code and the runtime agree on. The C++ side is
`runtime/include/vertex/abi.h`; the Go side is the constants in
`stdlib.go`, which `vsc/lower` and `vsc/internal/sil/gen` read. The two are
checked against each other by `build/abi_test.go`, which compiles
`abi.h` with vcx for every target and compares vcx's layout with the Go
numbers.

The shapes of metadata, value witness tables and existentials are
Swift's, because they are good ones and vsc already emitted them. The
symbols, the representation of `String` and `Array`, and the heap object
header are Vertex's own. Nothing here is frozen yet.

## Symbols

Every runtime entry point is `extern "C"` and starts `vertex_`. The
target's symbol prefix (`_` on Mach-O) goes in front, as for any C
symbol. The names are constants in `stdlib.go`, so the compiler and the
runtime spell each one once.

| Symbol | Signature | Is |
| --- | --- | --- |
| `vertex_alloc` | `(u64 size) -> HeapObject*` | an object with `size` zeroed bytes after the header, count one |
| `vertex_dealloc` | `(HeapObject*)` | an object's memory, once what it owned is released |
| `vertex_retain` | `(HeapObject*)` | add a reference; null is ignored |
| `vertex_release` | `(HeapObject*)` | drop one; at zero, `metadata->destroy` or free; null is ignored |
| `vertex_is_unique` | `(HeapObject*) -> bool` | the question copy-on-write asks |
| `vertex_error_box` | `(void* existential) -> HeapObject*` | a thrown `any Error`, moved into a box the error register carries |
| `vertex_error_contents` | `(HeapObject*) -> void*` | where a box's `any Error` is, for a catch to copy it out |
| `vertex_error_matches` | `(HeapObject*, const Metadata*) -> u64` | 1 when a box's error is a value of exactly that type, for `catch is T` and `catch let e as T` |
| `vertex_error_project` | `(HeapObject*) -> void*` | where a box's error value is: its buffer, or the value box it points at |
| `vertex_task_spawn` | `(void (*)(), void* context) -> void` | starts a task running the function, on a stack of its own; it runs when the current task next waits |
| `vertex_task_yield` | `() -> void` | lets every other runnable task run before this one carries on |
| `vertex_task_sleep` | `(u64 nanoseconds) -> void` | suspends the task until the deadline; outside a task, the thread sleeps |
| `vertex_task_run` | `() -> void` | runs tasks until the main task has finished, or none is left |
| `vertex_task_start` | `(void (*)(), void* context) -> HeapObject*` | starts a task as `vertex_task_spawn` does, and is a counted handle to it |
| `vertex_task_join` | `(HeapObject* handle) -> void` | suspends the task until the handle's task has finished; returns at once outside a task |
| `vertex_task_cell` | `(u64 size) -> HeapObject*` | a counted, zeroed object of that many bytes, where a task's result is kept |
| `vertex_task_cell_contents` | `(HeapObject* cell) -> void*` | where a cell's bytes are |
| `vertex_task_cell_typed` | `(const Metadata* type) -> HeapObject*` | a counted box for a value of that type, zeroed, whose end destroys the value |
| `vertex_task_cell_typed_contents` | `(HeapObject* cell) -> void*` | where a typed cell's value is |
| `vertex_task_wait_fd` | `(i32 fd, i32 events, i64 timeout) -> i32` | waits until the descriptor can be read (1) or written (2), or until timeout nanoseconds pass (negative: no deadline); 1 ready, 0 timed out. In a task the task waits and the executor runs everything else; outside one, or with no readiness registration, the thread waits |
| `vertex_task_start_detached` | `(void (*)(), void* context) -> HeapObject*` | starts a task as `vertex_task_start` does, on a worker of the pool: `Task.detached` |
| `vertex_task_hop` | `(u64 where) -> void` | suspends the task and resumes it on another executor, once its frames have unwound: 0 the main executor, 1 the pool, 2 the task's home (where it was started); on that executor already, a yield |
| `vertex_task_needs_hop` | `(u64 where) -> u64` | 1 when a hop there would move the task, 0 when it is there already or there is no task: what the compiler asks before each hop it emits |
| `vertex_task_on_main` | `() -> bool` | whether this thread is the main executor's |
| `vertex_task_assume_main` | `() -> void` | ends the program unless this thread is the main executor's: `MainActor.assumeIsolated` |
| `vertex_async_main` | `(void (*)(), void* context) -> void` | runs an async `main` as the first task |
| `vertex_async_main_status` | `(i32 (*)(), void* context) -> i32` | runs an async `main() -> Int32` as the first task, and is its status |
| `vertex_task_switch` | `(void** save, void* next) -> void` | assembly in the runtime's object: saves the callee-saved registers and SP into `*save`, restores them from `next` |
| `vertex_call_context` | `(void (*)(), void* context) -> void` | assembly in the runtime's object: calls the code with the context in the self register, as a function value is called |
| `vertex_call_context_status` | `(i32 (*)(), void* context) -> i32` | the same code, for an entry that returns a status |
| `vertex_string_literal` | `(u8* bytes, u64 count, bool) -> String` | a literal's String |
| `vertex_string_retain` / `_release` | `(u64 object)` | count the second word, if it is a reference |
| `vertex_string_count` | `(String) -> i64` | extended grapheme clusters |
| `vertex_string_concat` | `(String, String) -> String` | `+` |
| `vertex_string_equal` | `(String, String) -> bool` | `==`, canonical equivalence |
| `vertex_string_less` | `(String, String) -> bool` | `<`, by NFC scalar values |
| `vertex_describe` | `(void* value, Metadata*) -> String` | `String(describing:)` |
| `vertex_array_allocate` | `(i64 count, Metadata* element) -> {ArrayStorage*, u8*}` | storage and where its elements go |
| `vertex_array_count` | `(ArrayStorage*, Metadata*) -> i64` | `count` |
| `vertex_array_element` | `(i64 index, ArrayStorage*, Metadata*) -> u8*` | an element's address, bounds checked |
| `vertex_array_unique_elements` | `(ArrayStorage**, Metadata*) -> u8*` | where the elements are once the storage is the variable's alone: `withUnsafeMutableBufferPointer` |
| `vertex_array_repeating` | `(i64 count, u8* value, const Metadata*) -> ArrayStorage*` | an array of count copies of the value, which it takes: `[T](repeating:count:)` |
| `vertex_print` | `(ArrayStorage* items, String separator, String terminator)` | `print` |
| `vertex_read_line` | `(bool strippingNewline) -> String?` | `readLine`, nil as two zero words |
| `vertex_command_line_arguments` | `() -> ArrayStorage*` | `CommandLine.arguments` |
| `vertex_string_is_empty`, `vertex_array_is_empty` | `(…) -> bool` | `isEmpty` |
| `vertex_string_description`, `vertex_string_debug_description` | `(String) -> String` | `description`, `debugDescription` |
| `vertex_metadata_<Type>` | `FullMetadata` | the record for `Int`…`Double`, `Bool`, `String`, `Any`, and `Function`, which every function type shares |
| `vertex_string_from_utf8` | `(u8*, u64) -> String` | a String owning a copy of the bytes |
| `vertex_string_utf8` | `(String, u8* scratch, u64* count) -> u8*` | where a String's bytes are |
| `vertex_string_cstring`, `vertex_string_cstring_free` | `(String) -> u8*`, `(u8*)` | a NUL-terminated copy of the bytes, and letting it go: `withCString` |
| `vertex_string_from_cstring` | `(u8*) -> String` | `String(cString:)`: the bytes before the NUL |
| `vertex_string_utf8_array` | `(String) -> ArrayStorage*` | `utf8`, as an array of its bytes |
| `vertex_existential_is`, `vertex_existential_project` | `(Existential*, Metadata*) -> u64`, `(Existential*) -> void*` | whether an existential holds exactly a type, and where its value is: `x as? T` |

The bridge to Swift is a separate object, `swift.cpp`, linked only into
a program that calls a module `swiftc` built, together with the
libswiftCore stub:

| Symbol | Signature | Is |
| --- | --- | --- |
| `vertex_swift_string_to_swift` | `(String) -> SwiftString` | a Swift String with its own copy |
| `vertex_swift_string_from_swift` | `(SwiftString) -> String` | a Vertex String with its own copy |
| `vertex_swift_string_release` | `(u64 object)` | `swift_bridgeObjectRelease` |
| `vertex_swift_array_to_swift` | `(ArrayStorage*) -> void*` | a Swift array of the same elements |
| `vertex_swift_array_from_swift` | `(void*, Metadata* element) -> ArrayStorage*` | a Vertex array of the same elements, releasing the Swift one |
| `vertex_swift_release` | `(void*)` | `swift_release` |

Arrays cross only where the elements are the same bytes in both
languages: the integer and floating-point types and `Bool`.

A `String` argument is two `u64` arguments; a `String` result is a
two-word struct.

**Calling convention.** Arguments are never wider than a word: a
two-word value is two arguments. Results of two words are returned as a
16-byte struct, which AAPCS64 returns in `x0`/`x1` and vsc lowers the
same way. The Microsoft x64 convention returns one register and a
16-byte struct through a hidden pointer in `rcx`; vsc lowers every
result of more than one register that way on `x86_64-windows`, its own
functions' included, so a call into the runtime and a call between
Vertex functions look alike.

## Executors

Tasks run on executors, one per thread. The main executor is the main
thread's: it runs the main task, everything marked `@MainActor`, and
whatever the window system needs the main thread for. The others are a
pool of workers, one thread each, as many as `VERTEX_WORKERS` says or one
per processor but the main thread's; `VERTEX_WORKERS=0` is no pool, and
every task is the main executor's. The pool starts when something first
needs it.

A task belongs to one executor at a time, which is the only one that
runs it, and calls one *home*: the executor it was started on, or the
worker `Task.detached` gave it. Code that is not `@MainActor` runs at
home; `@MainActor` code runs on the main executor. Crossing is a hop, a
suspension the runtime completes only once the task's frames have
unwound, by handing the task to the other executor's inbox -- a lock-free
list the owner drains -- and waking it if it sleeps. The compiler asks
`vertex_task_needs_hop` and hops only when the answer is yes: on entry
to an async function that is `@MainActor` (to the main executor) or that
suspends somewhere (home), after every `await` (back to wherever that
function runs), and around a synchronous `@MainActor` call or property
awaited from elsewhere (there and back). A task with no pool, or one
whose home is the main executor, never moves, and every such question
costs a call and a compare.

Joining a task on another executor is safe: a handle's waiters are
under a spinlock, and each is woken on its own executor. Conformance
lookup, the heap and reference counts are shared between threads and
locked or atomic; a task's async frames are its own and travel with it.

## Heap objects

```
+0   metadata   const HeapMetadata*, or null
+8   refcount   u64; bit 63 is immortal
+16  stored properties
```

`HeapMetadata` is two words: `destroy(HeapObject*)`, which releases
what the object owns and frees it, and the object's dynamic type's
metadata, or null. An object whose metadata is null, or
whose `destroy` is null, is freed with nothing released. An immortal
object is never counted and never freed.

A class's dispatch table is its heap metadata: row 0 is `destroy`, row 1
the class's metadata, and the methods follow from row 2. The destroyer
runs the class's `deinit` and each superclass's, in that order, then
releases the stored references. `vertex_release` marks the object
immortal before calling it, so nothing the deinit does with `self` can
end the object a second time. Every class a
module declares has a table and a record, so an instance held as a
base class still describes itself by its own name. vsc's destroyer for a class releases the
`String`, `Array` and class references in its stored properties,
inherited ones included, and calls `vertex_dealloc`; a class with no
deinit and no references has null there.

## Type metadata

A metadata pointer points at the kind word. The value witness table
pointer is the word before it, so a record exported as a symbol is the
table pointer followed by the metadata, and a reference to a type is
the symbol plus `MetadataOffset` (8).

```
-8   ValueWitnessTable*
+0   kind       0x200 struct, 0x303 existential, …
+8   NominalTypeDescriptor*   (struct)
```

A struct's nominal descriptor is Swift's seven four-byte fields —
flags, then relative pointers to the module's descriptor, the name, the
accessor and the fields, then the field count and where the field
offsets begin (word 2, as four-byte offsets). The fields pointer is
Vertex's own record rather than Swift's reflection metadata:

```
+0   count          u64
+8   per field:     const char* name, const FullMetadata* type
```

It is null where a field's type has no metadata the module can point
at, and such a struct describes itself as its name and `()`.

Types declared nowhere get records in the module that needs them, with
Vertex's own kinds and value witnesses the runtime shares across every
instance (`runtime/generic.cpp`):

| Kind | Type | After the kind |
| --- | --- | --- |
| `0x202` | `Optional<T>` | payload metadata; u32 tag offset; u32 tag bytes; u64 value of the empty case |
| `0x800` | `Array<T>` | element metadata |
| `0x801` | a class | nominal descriptor; the value is one reference |

An Optional's empty case is a tag byte of 1 after the payload, or the
payload's spare representation: a null word for a reference or pointer,
2 in a Bool's byte.

The value witness table is eight functions — initializeBufferWithCopyOfBuffer,
destroy, initializeWithCopy, assignWithCopy, initializeWithTake,
assignWithTake, getEnumTagSinglePayload, storeEnumTagSinglePayload —
then `size`, `stride`, `flags` (u32, at 0x50) and the extra inhabitant
count. The low byte of `flags` is the alignment mask; bit 16 is
non-POD, bit 17 is non-inline.

For a struct that owns nothing, vsc points every copy at one memcpy and
destroy at a function that returns. For a struct holding `String`,
`Array` or class references — directly or through nested structs —
copy retains each reference, destroy releases each, and the struct is
marked non-POD.

## Existentials

`Any` is four words: a three-word inline buffer, then the metadata. A
protocol existential adds the witness table as a fifth. A value that
does not fit the buffer lives in a heap box whose pointer is the
buffer's first word, and the box is released with `vertex_release`.

## String

Two words. The top byte of the second says which of three forms it is:

| Top byte | Form | First word | Second word |
| --- | --- | --- | --- |
| `0x00` | native | count (48 bits) \| flags | `StringStorage*`, counted |
| `0x40` | literal | count \| flags | address of immortal bytes |
| `0xE0 \| n` | small, `n` ≤ 15 | bytes 0–7 | bytes 8–14 in the low seven bytes |

Flag bit 63 of the first word says every byte is ASCII. Both words zero
is no String -- no native string has a null storage pointer, and the
empty string is small -- and is how `String?` writes nil.
`StringStorage` is a heap object, a `u64` capacity, then the bytes at
+24. Only a native string's second word is a reference, which is why
retain and release go through `vertex_string_retain` and
`vertex_string_release` rather than the object ones.

The bytes are valid UTF-8. `count` is extended grapheme clusters by
UAX #29; `==` is canonical equivalence and `<` orders by the scalar
values of the NFC forms, both by UAX #15. The tables are generated from
UCD 17.0.0, which is the Unicode version Swift 6.3 implements, by
`internal/gen/unicode`.

## Array

One word: `ArrayStorage*`, never null.

```
+0   HeapObject   metadata is the runtime's array destroyer
+16  count        i64
+24  capacity     i64
+32  element      const Metadata*
+40  elements, each a stride of the element type
```

The empty array is one immortal global. When the last reference goes,
every element is destroyed through the element type's witnesses.

## Dictionary and Set

One word each: a `HashTable*`, never null. Open addressing with linear
probing, a power-of-two bucket count at most three-quarters full, and
deletion by backward shift, so there are no tombstones. A Set is a table
whose value type is null. The empty table is one immortal global,
`vertex_empty_hash_table`.

```
+0   HeapObject    metadata is the runtime's table destroyer
+16  count         i64
+24  buckets       i64
+32  key           const Metadata*
+40  value         const Metadata*, null for a Set
+48  keysOffset    u64, from the start of the table
+56  valuesOffset  u64
+64  used          u8 per bucket, then the keys, then the values
```

Keys and Set elements are hashed with SipHash-1-3 under a per-process
seed from the platform's entropy, so iteration order -- and therefore the
order `print` writes entries in -- differs between runs, as Swift's does.
The runtime hashes and compares the core's own types: the integers,
`Bool`, `Float`, `Double` (with `-0.0 == 0.0`) and `String` (by the
scalars of its NFC form, so canonically equal strings are one key).

Collection operations pass values by address with their type's metadata
after them:

| Symbol | Signature | Is |
| --- | --- | --- |
| `vertex_array_append` | `(ArrayStorage** slot, void* value, Metadata* element)` | `append(_:)`, taking the value |
| `vertex_array_append_contents` | `(ArrayStorage**, ArrayStorage* other, Metadata*)` | `append(contentsOf:)` |
| `vertex_array_assign` | `(ArrayStorage**, i64 index, void* value, Metadata*)` | `a[i] = v`, taking the value |
| `vertex_array_element_for_write` | `(ArrayStorage**, i64 index, Metadata*) -> void*` | where `a[i]` is, the storage made unique: `a[i].x = v`, `a[i].mutate()` |
| `vertex_array_insert` | `(ArrayStorage**, void* value, i64 index, Metadata*)` | `insert(_:at:)` |
| `vertex_array_remove_at` / `_remove_last` | `(ArrayStorage**, [i64 index,] void* out, Metadata*)` | `remove(at:)`, `removeLast()` |
| `vertex_array_pop_last` | `(ArrayStorage**, void* out, OptionalMetadata*)` | `popLast()`: the last element, or nil |
| `vertex_array_remove_all` | `(ArrayStorage**, Metadata*)` | `removeAll()` |
| `vertex_array_first` / `_last` | `(ArrayStorage*, void* out, OptionalMetadata*)` | `first`, `last` |
| `vertex_array_contains` | `(ArrayStorage*, void* value, Metadata*) -> bool` | `contains(_:)` |
| `vertex_hash_table_empty` | `() -> HashTable*` | `[:]`, `[]` as a Set |
| `vertex_hash_table_count` / `_is_empty` | `(HashTable*)` | `count`, `isEmpty` |
| `vertex_dictionary_get_default` | `(HashTable*, key borrowed, default taken, out, valueMeta)` | `d[k, default: v]`: the value, or the default |
| `vertex_hash_table_next` | `(HashTable*, Int from) -> Int` | the first bucket in use at or after `from`, or -1: a for-in's step |
| `vertex_hash_table_key_at` / `vertex_dictionary_value_at` | `(HashTable*, Int bucket, out, meta)` | copies a bucket's key or value out |
| `vertex_dictionary_insert_literal` | `(HashTable**, void* key, void* value, Metadata* key, Metadata* value)` | one literal entry, taking both; a repeated key traps |
| `vertex_dictionary_get` | `(HashTable*, void* key, void* out, OptionalMetadata* value)` | `d[k]` |
| `vertex_dictionary_set` | `(HashTable**, void* key, void* optional, Metadata* key, OptionalMetadata*)` | `d[k] = v`; nil removes |
| `vertex_dictionary_remove` | `(HashTable**, void* key, void* out, Metadata* key, OptionalMetadata*)` | `removeValue(forKey:)` |
| `vertex_set_insert` / `_contains` / `_remove` | `(HashTable**/HashTable*, void* element, …)` | `insert`, `contains`, `remove` |

Every mutation copies shared storage first (copy on write), through the
element's witnesses; storage the caller holds alone is changed in place.

## Descriptions

`print` and `String(describing:)` produce exactly what Swift's do for
the integer types, `Bool`, `String`, `Float` and `Double` — the
shortest digits that round-trip, in fixed notation unless the magnitude
exceeds 2^53 (2^24 for `Float`) or the value is below 10^-4, in which
case `d.ddde±XX` — and for:

| Value | Written |
| --- | --- |
| a struct | `Point(x: 1, y: 2.5)`, each field its debug description |
| an Optional | `Optional(3)`, or `nil` |
| an Array | `[1, 2, 3]`, each element its debug description |
| a class instance | `main.Holder`, by its dynamic type |

A debug description quotes and escapes a `String` as Swift's
`debugDescription` does, and qualifies a struct by its module.
