# tests/vsx

The test ladder for `.vsx` (v2) — Vertex's `.tsx`-inspired declarative UI and markup file format.

| Folder | What | Checked by |
| --- | --- | --- |
| `syntax/` | `.vsx` v2 syntax and component examples (`counter.vsx`, `tasks.vsx`, `async.vsx`). Must parse with no diagnostics and contain markup. | `parser/markup_test.go` (`TestMarkupFiles`) |
| (in the test) | Snippets of the grammar JSX/TSX and `.vsx` share, compared live with `tsc --jsx react`. | `parser/tsx_oracle_test.go` (`TestTSXOracle`) |
| `run/` | `.vsx` v2 programs compiled with the built-in `.vsx` prelude (`analyzer/vsx_prelude.vs`), linked, executed natively, and compared with the `.out` file beside each. | `build/vsx_test.go` (`TestVSXRun`) |
| `errors/` | `.vsx` v2 programs that must be rejected: each line marked `// want "part of the message"` must receive that diagnostic, and no other line may fail. | `build/vsx_test.go` (`TestVSXErrors`) |

```bash
cd build && go test -v . -run TestVSX
```

## Built-in `.vsx` v2 Prelude (`analyzer/vsx_prelude.vs`)

Every `.vsx` compilation unit automatically receives the built-in `.vsx` v2 prelude:

- **Core UI Types**: `Element`, `Node`, `Props`, `Style`, `CSSValue`, `Ref<T>`, `Context<T>`, `Component<P, S>`, `EmptyState`
- **Built-in Components**: `Fragment`, `Suspense`, `StrictMode`, `Profiler`
- **Hooks**: `useState`, `useReducer`, `useEffect`, `useLayoutEffect`, `useInsertionEffect`, `useRef`, `useImperativeHandle`, `useMemo`, `useCallback`, `createContext`, `use`, `useContext`, `useId`, `useTransition`, `startTransition`, `useDeferredValue`, `useSyncExternalStore`, `useOptimistic`, `useActionState`, `useFormStatus`
- **DOM Events & Elements**: `Event`, `MouseEvent`, `KeyboardEvent`, `ChangeEvent`, `PointerEvent`, `FocusEvent`, `FormData`, `HTMLElement`, `HTMLInputElement`, `HTMLButtonElement`, `HTMLDivElement`, `HTMLFormElement`, `HTMLAttributes`
- **Optional Namespaces**: `ui.*` and `dom.*` resolve to the same built-in symbols for optional explicit qualification.
