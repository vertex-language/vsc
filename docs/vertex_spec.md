# The Vertex Language

Vertex is Swift with a small set of opt-in additions. This document is
the delta: every production Vertex adds to or changes in
[the Swift grammar](swift_grammar.md), and the rules that go with
them. Its notation is that document's.

Nothing here changes the meaning of a program that does not use it. A
valid Swift file is a valid Vertex file.

## 1. Principles

**Swift is the base.** Every addition is written down in this
document. Anything else that differs from Swift is a bug rather than a
design decision — a program Swift accepts and Vertex rejects, one
Swift rejects and Vertex accepts, or one the two read differently.

**Additions are grammar, not a second model.** Vertex adds no second
notion of what a module is, what a namespace is, how a name resolves,
or what a method is. Swift's answers stand; what these productions add
is a way to write things Swift has no syntax for.

**A module's identity is an identifier.** It is written into every
symbol the module defines, so anything that names a module — a folder,
a path, a clause — must resolve to one identifier.

## 2. Types

### 2.1 Primitive spellings

Vertex admits lowercase spellings of the primitive types. Each is an
alias denoting exactly the type its capitalised Swift name does; the
two are one type, not two.

```text
VertexTypeName: (one of)
bool char string void never any
int int8 int16 int32 int64
uint uint8 uint16 uint32 uint64
float float32 double float64

```

| Vertex | Swift |
| --- | --- |
| `bool` | `Bool` |
| `int`, `int8`, `int16`, `int32`, `int64` | `Int`, `Int8` … `Int64` |
| `uint`, `uint8`, `uint16`, `uint32`, `uint64` | `UInt`, `UInt8` … `UInt64` |
| `float`, `float32` | `Float` |
| `double`, `float64` | `Double` |
| `string` | `String` |
| `char` | `Character` |
| `void`, `never`, `any` | `Void`, `Never`, `Any` |

Swift's names are resolved first. A program written only in those
reads exactly the universe `swiftc` does.

## 3. Declarations

### 3.1 Package declaration

```text
PackageDeclaration:
package Identifier

```

Names the module the file belongs to, which Swift leaves to the build
system's `-module-name`.

- The name must be an identifier, upper or lower case, because it is
  mangled into every symbol the module defines.
- Where a file declares none, the module's name is its folder's
  (§5.1), or the compiler's `-module` flag for a file compiled
  directly.
- `main` in module `main` is the program's entry point; every other
  module's `main` is an ordinary function.

**Disambiguation.** Swift spends `package` on an access-level
modifier. The two never look alike: `package` followed by an
identifier that does not begin a declaration is this clause, and
`package` followed by a declaration keyword or another modifier is the
access level. Both remain available and nothing is newly reserved.

### 3.2 Import declarations

Swift's identifier form is unchanged. Vertex adds a string form, which
names a folder of source rather than a module built elsewhere.

```text
ImportDeclaration:
[AttributeList] [AccessLevelModifier] import [ImportKind] ImportPath
[AttributeList] [AccessLevelModifier] import ImportSpec
[AttributeList] [AccessLevelModifier] import '(' {ImportSpec} ')'

ImportSpec:
[Identifier] StringLiteral

```

- The parenthesised group stands for one import each. It is
  punctuation and carries no other meaning.
- An `Identifier` before the string binds the module under that name
  instead of the path's last segment.
- The path must be a plain string literal: one run of text, no
  interpolation.
- `ImportKind` is meaningful only with the identifier form.

Resolution is §5.2; what a folder is read as is §5.3.

### 3.3 Receiver methods

A method may be written outside its type's body, with the receiver
named.

```text
FunctionDeclaration:
[AttributeList] [DeclarationModifiers] func [ReceiverClause] FunctionName
    [GenericParameterClause] FunctionSignature [GenericWhereClause]
    [FunctionBody]

ReceiverClause:
'(' Identifier ':' [OwnershipModifier] Type ')'

OwnershipModifier: (one of)
borrowing consuming inout __shared __owned

```

A receiver clause may stand only between `func` and the name, a
position nothing else may occupy: a function is named by an identifier
or an operator, and `(` is neither.

The clause takes no argument label. It reads like a parameter and is
not one.

**Meaning.** A receiver method is an extension member. These are the
same declaration:

```text
func (v: borrowing vec2) length() -> float32 { … }

extension vec2 { func length() -> float32 { … } }

```

The ownership modifier is what Swift spells on the method:

| Receiver kind | `borrowing` | `inout` | `consuming` |
| --- | --- | --- | --- |
| `struct`, `enum` | ordinary method | `mutating func` | `consuming func` |
| `class` | ordinary method | *not admitted* | `consuming func` |

`borrowing` is the default and may be omitted.

A class receiver is a reference: a method that changes a property
changes the object without the receiver being mutable. Swift has no
`mutating` method on a class, and `inout` would have to mean rebinding
the reference, which no method does — so an `inout` receiver on a
class is rejected.

**What follows from being an extension member**, none of it a choice
Vertex makes differently:

- Statically dispatched, and not in the type's table. A receiver
  method cannot be overridden.
- May not add stored properties.
- May be written for any nominal type, including an imported one.
- May satisfy a protocol requirement.

**The receiver's name** is in scope in the body and denotes what
`self` denotes. It is a scope entry, not a parameter: the method
already receives `self`. Members remain reachable by implicit `self`
as well.

### 3.4 Execution modifiers

```text
FunctionSignature:
'(' [ParameterList] ')' [async] [ThrowsClause] [ExecutionModifier]
    [FunctionResult]

ExecutionModifier: (one of)
kernel graph

```

`kernel` marks a data-parallel unit, `graph` a node in a dataflow
graph. Both are reserved: they parse and typecheck, and no backend
generates code for either.

The modifier stands between the throws clause and the result arrow,
where nothing else in the grammar may. Both words stay contextual — a
function, parameter or variable may still be named either.

A function carrying one is **refused at lowering**, not lowered as an
ordinary function. Compiling a `kernel` to a CPU function would return
a plausible answer and hide the fact that the modifier did nothing.

## 4. Calls

### 4.1 Argument labels

Labels are Vertex's compatibility surface rather than its grammar, and
are optional in both directions: a label the declaration asks for may
be left out, and a label may be written where the declaration says
`_`.

```text
func addUp(a: int32, b: int32) -> int32 { … }

addUp(1, 2)         // no labels
addUp(a: 1, b: 2)   // labels

```

A written label must still name the parameter, by its label or by the
name its body uses. A label naming no parameter is an error.

**Labels remain significant in three places.**

1. **Overload resolution.** Declarations may differ only by label —
   `label(a:)` and `label(b:)` are two functions and two symbols. A
   call is resolved with the strict rule first, and only a call that
   rule rejects is retried with labels optional. Where that leaves
   more than one candidate the call is ambiguous, and is reported
   rather than guessed.
2. **A short argument list.** Where fewer arguments are given than
   there are parameters, the label is what says which parameters are
   supplied and which take their defaults.
3. **A variadic.** The label is what says where the variadic list
   stops: it takes every unlabelled argument, and a labelled argument
   after it belongs to the parameter of that name.

So labels are optional wherever the argument count matches the
parameter count, and required where it does not.

## 5. Modules and packages

### 5.1 Naming

A module's name is, in order:

1. the `package` declaration in its files, where one is written;
2. otherwise the last segment of the path that imported it, which is
   its folder's name;
3. otherwise the `-module` flag, defaulting to `main`.

The name must be an identifier. A directory whose name is not one —
`my-lib` — needs a `package` declaration to be importable.

### 5.2 Path resolution

| Path | Resolved against |
| --- | --- |
| `"std/fmt"`, `"acme/geometry"` | each package root, in order |
| `"./geometry"`, `"../shared"` | the directory of the importing file |

A path beginning `./` or `../` is relative; every other path is looked
for under the package roots, which are the compiler's `-P` flags
followed by the entries of `VERTEXPATH`.

The package roots are where a package manager places what it
downloads. Fetching is not part of the language: a downloaded
repository is a folder like any other once it has landed.

`-I` is a separate search path and is unchanged. It finds modules
already *built* — a `.vertexinterface` or a `.swiftinterface` — which
is how a library built by `swiftc` is reached. The package roots hold
source.

### 5.3 What a folder is

Every `*.vs` file in the named directory is that module. Subdirectories
are not part of it and are not submodules.

A folder's files are read the way a module interface is: parsed, and
checked for what they declare. Compiling a program compiles each
folder it imports as its own module and links the results, so a
program of several modules is one command.

## 6. Conformance

| Addition | § | State |
| --- | --- | --- |
| Primitive spellings | 2.1 | complete |
| Package declaration | 3.1 | complete |
| Import declarations | 3.2 | complete |
| Receiver methods | 3.3 | `borrowing` and `consuming` |
| Execution modifiers | 3.4 | parsed and refused, by design |
| Argument labels | 4.1 | complete |
| Modules and packages | 5 | complete |

An `inout` receiver (§3.3) typechecks and is refused at lowering. It
is blocked by a limitation that is not the receiver's: `self` is
passed by value and never `@inout`, so no `mutating` method can write
to its receiver, receiver clause or not. See `TODO.md`.

## 7. Open questions

- **Two modules may share a name.** A rename at the import changes
  what one file calls a module, not what the module's symbols are
  mangled with, so linking two modules both named `fmt` is a
  duplicate-symbol error either way. The `package` declaration is the
  only thing that changes identity. Erroring at the import is the
  likely answer, and it means one program cannot use two same-named
  modules.
- **A local silently shadows a module.** A bare name is read as a
  module only where the local scope does not already have it, so
  `let geometry = …` hides `import "./geometry"` with no diagnostic.
  Lowercase module names make it likelier. A warning would cost
  little.
- **Where may a receiver method be written?** Swift lets an extension
  sit anywhere in its module, and matching that is simplest.
- **Should `self` remain usable beside the receiver's name?** Allowing
  both keeps a mixed file readable; allowing only the name makes a
  receiver method impossible to move into a type body unedited.
- **Generic receivers.** `func (s: borrowing Stack<T>) peek() -> T`
  needs its parameters bound the way `extension Stack` binds them.
- **Should the package roots and `-I` merge?** They answer different
  questions today — source against built modules — and keeping them
  apart is what makes `swiftc` interop and folder imports independent.
- **Source or builds under the package roots?** Source is assumed, so
  a program builds its dependencies. Caching objects beside them is
  where something must decide what is stale.
