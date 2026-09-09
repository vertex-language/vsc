**Overview**
Vertex is an independent compiler targeting the Swift language specification plus custom additions (`docs/vertex_spec.md`), verified against Apple's `swiftc`. The compiler currently has zero known code generation bugs that produce incorrect runtime values. Current development focuses on adding diagnostic errors for unhandled attributes, lowering missing Swift language features, and implementing C/Objective-C interop.

---

**Pending Issues**

* **Bugs & Unhandled Attributes** *(Invalid code or attributes currently accepted without error)*:
* **Operator precedence:** `r?.value!` binds as `(r?.value)!` instead of failing with a syntax error on `r?.(value!)`.
* **Constant overflow:** Compile-time arithmetic overflow traps at runtime instead of being caught as a build error.
* **Ignored calling convention attribute:** `@convention(c)` on a function *type* parses cleanly and lowers to `@convention(thin)`, so a value declared that way is not a C function pointer.
* **Ignored memory qualifiers:** `weak` and `unowned` parse cleanly without memory management semantics, resulting in memory leaks.


* **Unsupported Language Features** *(Valid Swift rejected at compile time)*:
* **Control flow:** `defer`, `throw`, and `do`/`catch`.
* **Functions & closures:** Capturing closures, failable initializers, custom operators, and `super` calls.
* **Declarations & members:** Subscripts, global variables/constants, stored static properties, and computed properties on subclassed classes.
* **Objective-C:** `@objc` is rejected outright; it needs the Objective-C runtime, message dispatch, and class metadata, none of which exist.
* **C entry points beyond scalars:** `@_cdecl` is rejected for any signature whose parameters or result do not cross in a register.
* **Types & data layout:** `Bool?` and other optionals relying on spare bits/extra inhabitants, tuple pattern matching in `switch`, and payload enums exceeding one word in width.
* **Protocols & extensions:** Protocol extensions cannot resolve protocol requirements or expose default implementations to conforming types.
* **Top-level code:** File-scope executable statements are rejected (requires an explicit `func main`).


* **Unimplemented Subsystems:**
* Concurrency (`async`/`await`, actors).
* C-compatible pointer types (`UnsafePointer`, `UnsafeMutablePointer`, `OpaquePointer`, etc.). These are what block every C signature that is not scalars, so they gate the rest of C interop.
* Clang importer (bridging headers, C module maps, and Objective-C runtime dispatch).



---

**Fixed Issues**

* **Runtime & Code Generation Fixes:**
* Bitshifts (`<<`, `>>`) for counts exceeding operand width or negative values now match Swift semantics instead of emitting undefined CPU instructions.
* Subclass initializers now correctly initialize inherited stored properties.
* Ternary branches now preserve uniform byte widths at join points (e.g., `c ? nil : v`).
* Optional assignments no longer overwrite tag bytes with payload data.
* Property accessor generation now parses `get`/`set` by keyword rather than declaration order.
* Property observers (`willSet`, `didSet`) now execute on mutation.
* Custom operators on literals evaluate to the operator’s defined return type rather than defaulting to the left operand.
* Optional chaining expressions now return correct types to trailing nil-coalescing (`??`) operators.


* **Type Checker & Lowering Fixes:**
* Support for multiple unwrapping conditions in `guard let` and `while let`.
* Added missing operator precedence groups (correcting expression folding such as `&` vs `==`).
* Support for masking arithmetic operators (`&+`, `&-`, `&*`, `&<<`, `&>>`).
* Equality comparisons for optionals against other optionals, values, and `nil`.
* Unqualified member access for inherited properties without explicit `self`.
* `mutating` methods now pass value-type receivers by reference (`inout`) rather than by value.
* Enum `.rawValue` extraction.
* Default argument support in memberwise initializers and bare `return` in initializers.
* Lowercase primitive type conversions (e.g., `int32(x)`).


* **Interoperability:**
* `@_silgen_name("f")` now names the symbol at the definition and at every call, so a body-less declaration is a declaration of somebody else's function rather than one that traps. C functions are callable without an importer.
* `@_cdecl("f")` now emits a C-convention thunk beside the function, matching swiftc, so the symbol is exported and Vertex can still call the function by its own name.
* Functions declared with no body and no symbol attribute are rejected instead of silently lowering to a trap.
* Duplicate object-file symbols are caught in both declaration orders rather than producing one function with two bodies.
* New `tests/cinterop/` corpus: a C program built by clang links against a module built by this compiler and calls into it, and Vertex calls back into C.


* **Semantic Validation Fixes:**
* Properly rejects `mutating` modifiers on class methods.
* Properly rejects stored properties declared inside enums.