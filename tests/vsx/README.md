# tests/vsx

The ladder for `.vsx`, Vertex with markup (`proposed_vsx.md` on the
Desktop is the design). Markup has no oracle the way the Swift ladder has
swiftc, so each test writes down what it expects.

| Folder | What | Checked by |
| --- | --- | --- |
| `syntax/` | The RFC's own examples. They must parse, with no diagnostics, and hold markup. | `parser/markup_test.go` |
| (in the test) | Snippets of the grammar JSX and markup share, compared with what `tsc --jsx react` makes of them. | `parser/tsx_oracle_test.go` |
| `run/` | Programs compiled with `prelude.vs`, run, and compared with the `.out` beside each. | `build/vsx_test.go` (`TestVSXRun`) |
| `errors/` | Programs that must be refused: each line marked `// want "part of the message"` has to get that error, and no other line may get one. | `build/vsx_test.go` (`TestVSXErrors`) |

```bash
cd build && go test . -run TestVSX
```

## `prelude.vs`

Markup is checked in its *check form* (RFC §9.2): an element is rewritten
as the ordinary calls it means, and those are checked and lowered in its
place.

```
<Card title="T">…</Card>          Card(title: "T", children: { […] })
<p class="x" onClick={…}>…</p>    component.Element("p", [
                                      component.Attribute.Static("class", "x"),
                                      component.Attribute.On("click", dom.MouseEvent.self, { _ in … }),
                                  ], […])
<>…</>                            component.Fragment([…])
```

`component` and `dom` are what `ui/component` and `web/dom` will be
imported as. Until they exist, `prelude.vs` declares the same API itself:
it builds a tree of `Node`s, renders it to HTML with `Html()`, and keeps
each element's handlers so a test can `Fire("click")` them.

Two vsc gaps shape the prelude, and it says where: a cast to a class nested
in an enum answers false, and a type reached through a typealias in an enum
is named by the alias. So the event classes are top-level, under the names
`dom` gives them.

## Numbering

`run/` and `errors/` are numbered separately, in the order they were
added, one thing per file: elements, components, control flow, handlers,
the RFC's tasks app; names, props.
