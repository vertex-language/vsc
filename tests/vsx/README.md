# tests/vsx

The `.vsx` ladder: one program of ordinary `.vsx` per file, numbered `001`–`040`.
Each is compiled with the built-in `.vsx` prelude (`analyzer/vsx_prelude.vs`),
linked, run natively, and its standard output compared with the file's own
`// want:` lines, as the kernel ladder's files are:

```vertex
print((<p>{name}</p>).Html())
// want: <p>Hello</p>
```

```bash
cd build && go test -run TestVSX .
cd build && VSX_UPDATE=1 go test -run TestVSXRun/041 .   # write a new program's want lines; read them before keeping them
```

Every file also has to parse (`parser/markup_test.go`).

| | |
| --- | --- |
| 001–006 | elements, components, control flow, handlers, a task list, hooks with a Props struct and a class component |
| 007–012 | text and escaping, fragments, conditionals (`if`, `switch`, `&&`, `?:`), lists, attributes, inline styles |
| 013–024 | handlers, `useState`, `useReducer`, context, Props structs, render props, generic components, refs, `useMemo`, `useEffect` and its cleanup, class components |
| 025–028 | the language under the hooks: `any Equatable` deps, overloads told apart by a closure's body, implicitly unwrapped optionals, a class inheriting a generic class |
| 029–040 | programs: a task app, SVG, forms, whitespace, composition, `useId`, keyed rows, tabs, `Suspense`, optionals, a cart, a dashboard |

The prelude is what a `.vsx` program runs on where `ui` cannot be found. Where it
can, the program imports `ui/component` and `web/dom` implicitly and runs on them
(`vsximports.go`).
