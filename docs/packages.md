# Packages

A package is a folder. Every `.vs` file in it is the package, and a `.cpp`
file in it is the package's native half. Nothing lists the files, and there
is no manifest to keep in step with the folder.

```
geo/                         ← repository: module github.com/you/geo
├── vs.mod
├── shape/                   ← import "github.com/you/geo/shape"
│   ├── shape.vs             ←   package shape
│   ├── shape.cpp            ←   export module geo.shape;   (its native half)
│   └── shape_windows.cpp    ←   only built for Windows
├── fastmath/                ← import "github.com/you/geo/fastmath"
│   └── fastmath.cpp         ←   a package of C++ alone
├── testdata/                ← files the programs read; never built
└── cmd/
    └── area/main.vs         ← vsc run area
```

## Creating a package

1. **Make the folder and a `vs.mod`.** The module path is where the repository
   lives, and `platform` is the oldest OS it runs on:

   ```
   module github.com/you/geo

   vertex 0.9
   platform macos 14
   ```

2. **Write the package.** One `package` name per folder, the folder's own
   name. `public` is the API; everything else stays inside the package:

   ```swift
   // shape/shape.vs
   package shape

   /// A circle, by its radius.
   public struct Circle {
       public var Radius: float64

       public init(radius: float64) {
           Radius = radius
       }
   }

   public func (c: borrowing Circle) Area() -> float64 {
       return 3.141592653589793 * c.Radius * c.Radius
   }
   ```

3. **Add programs in `cmd/<name>/main.vs`,** tests included, and run them by
   name from anywhere in the repository:

   ```swift
   // cmd/area/main.vs
   package main

   import "github.com/you/geo/shape"

   print(shape.Circle(radius: 2).Area())
   ```

   ```bash
   vsc run area
   ```

   A bare name is always `cmd/<name>`, even when a package folder has the same
   name. `./shape` names the folder.

4. **Commit `vs.sum`** once a dependency has been fetched. vsc records each
   dependency's hash there and checks it on every later fetch.

## Best practices

- **Name exported API in `UpperCamelCase`:** `fs.Open`, `png.Decode`,
  `window.Create`. Keep internals `lowerCamelCase`. A reader can tell from the
  call site what is part of the package's contract.
- **Share across the repository with `package`,** not `public`: a helper
  `db/sqlite` needs from `db/sql` is `package func`, so it isn't part of
  either package's API. Anything unmarked is internal, and another package
  that names it gets "inaccessible due to 'internal' protection level".
- **Keep one idea per folder.** Subfolders are separate packages
  (`rdp/codec/planar`), so a large package splits into small ones that import
  each other, and each is testable alone.
- **Put tests and tools in `cmd/`.** A check program prints `ok` lines and
  exits non-zero on failure (`vsc run check`). A program that talks to the
  network or a device gets its own name (`hub-live-test`), so the offline
  checks stay fast.
- **Put sample input in `testdata/`.** vsc never builds it.
- **Use file suffixes for platform code, not `#if` around whole files:**
  `_darwin`, `_linux`, `_windows`, `_android`, `_ios`, `_posix` (everything
  but Windows), `_arm64`, `_amd64`, or both (`_linux_amd64`). This works for
  `.vs` and `.cpp` alike.
- **Reach for C++ last.** Parsing, validation, defaults and data structures
  belong in Vertex. Native code is for the OS call that Vertex can't make.

## Standard packages

A short path imports a package from `github.com/vertex-language`, so
`import "net/http"` is the `http` folder of the `net` repository. The standard
packages are `archive`, `cli`, `compress`, `crypto`, `db`, `encoding`, `fs`,
`hash`, `image`, `io`, `log`, `math`, `model`, `net`, `nn`, `os`, `remote`,
`tensor`, `text`, `time` and `ui`, plus the function packages of the `gpu`
repository. Any other package is imported by its full path
(`"github.com/you/geo/shape"`).

The first build that needs a package fetches it into the package cache
(`VERTEXCACHE`, or `vertex` in this machine's cache directory). Cloning is
built in, so there's no need to have git installed. Later builds read the
cache.

| Flag | Does |
| :--- | :--- |
| `-replace path=dir` | build a package from a local checkout |
| `-offline` | never fetch; use the cache as it is |
| `-update` | fetch again, even if the cache has the package |

## Working on several repositories

A `vs.work` above the checkouts makes every build use them in place of the
cache:

```
vertex 0.9

use (
    ./fs
    ./io
    ./net
    ./geo
)
```
