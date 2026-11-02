# VSX

A `.vsx` file is Vertex source with markup in it. It sits beside the `.vs` files of its package and is compiled with them by `vsc`. Markup is an expression: `<p>Hi</p>` is a value of type `Element`.

```vertex
package main

func Counter(start: int) -> Element {
    let (count, setCount) = useState(start)
    return <button onClick={setCount(count + 1)}>Clicked {count} times</button>
}
```

This document says what `.vsx` accepts, what it means, and what it runs on. What is not written here is not supported.

---

## 1. Files and imports

A `.vsx` file imports three packages without writing them:

| Package | Bound as | What it gives |
| :--- | :--- | :--- |
| `ui/component` | `component` | what markup lowers to, the hooks, `Mount`, and the names in §1.1 |
| `web/dom` | `dom` | the events handlers are given (`dom.MouseEvent`, …) |
| `ui/viewport` | `viewport` | `<Viewport>`, a box something else draws into (§10) |

They are imported where `vsc` can find them from the file's folder. Where it cannot (a program with no `ui` to build on), `vsc` compiles the file against its built-in prelude instead (§9), which has the same names.

A file may still write either import; it is the same import.

### 1.1 Names available without qualification in a `.vsx` file

| Kind | Names |
| :--- | :--- |
| Types | `Element` (= `Node`), `Node`, `Props`, `EmptyProps`, `EmptyState`, `Style`, `CSSValue`, `Ref<T>`, `Context<T>`, `Component<P, S>`, `SetState<T>`, `Children` (= `() -> Node`) |
| Element types | `HTMLElement`, `HTMLInputElement`, `HTMLButtonElement`, `HTMLDivElement`, `HTMLSpanElement`, `HTMLFormElement` — all the element node type |
| Hooks | `useState`, `useReducer`, `useRef`, `useEffect`, `useLayoutEffect`, `useInsertionEffect`, `useMemo`, `useCallback`, `useId`, `useTransition`, `startTransition`, `useDeferredValue`, `createContext`, `use`, `useContext` |
| Components | `Suspense`, `StrictMode`, `Profiler`, `Viewport` (`ui` only, §10) |
| Functions | `Focus`, `Bounds` (`ui` only, §8.4), `renderToString` |

Event types are written through `dom`: `dom.MouseEvent`, `dom.KeyboardEvent`. A plain `.vs` file has none of this: it writes `import "ui/component"` and names `component.Node`.

---

## 2. Markup

### 2.1 Grammar

Non-terminals are `CamelCase`, terminals are `monospace` or quoted, `[x]` is optional, `{x}` repeats.

```text
PrimaryExpression:
    ...                      (every Vertex primary expression)
    MarkupElement
    MarkupFragment

MarkupElement:
    '<' ElementName [GenericArguments] {Attribute} '/' '>'
    '<' ElementName [GenericArguments] {Attribute} '>' {Child} '<' '/' ElementName '>'

MarkupFragment:
    '<' '>' {Child} '<' '/' '>'

ElementName:
    MarkupIdentifier                       div   color-picker   Card
    MarkupIdentifier ':' MarkupIdentifier  svg:rect
    ElementName '.' MarkupIdentifier       kit.Card   ui2.Nav

MarkupIdentifier:
    an identifier, which may also contain '-'

Attribute:
    AttributeName                          bare: the value true
    AttributeName '=' StringValue
    AttributeName '=' '{' Code '}'
    AttributeName '=' MarkupElement        icon=<i>*</i>
    '{' '...' Expression '}'               a spread

AttributeName:
    MarkupIdentifier
    MarkupIdentifier ':' MarkupIdentifier  xlink:href

StringValue:
    '"' characters other than '"' '"'
    "'" characters other than "'" "'"

Child:
    Text                                   characters other than '{' '<' '>' '}'
    MarkupElement
    MarkupFragment
    '{' Code '}'
    '{' comment '}'                        {/* nothing */}
```

`Code` is what a closure's body holds: a closure signature with `in` (`e in …`) and statements, or statements alone (§3).

A string value has no escapes. In a string value and in text, HTML character references are decoded: named ones (`&amp;` `&lt;` `&gt;` `&quot;` `&nbsp;` …) and numeric ones (`&#65;` `&#x42;`).

### 2.2 Whitespace

Text follows JSX's rules. A line of text is trimmed; lines are joined with one space; a line made only of whitespace, and whitespace that runs up to or away from a line break beside an element or braces, is dropped. Spaces inside a line are kept.

```vertex
<p>
    Hello,
    {name}!   How are
    you?
</p>
// <p>Hello,World!   How are you?</p>     with name = "World"
```

### 2.3 Where `<` starts markup

`<` starts markup where an expression starts and the next token is an identifier or `>`. After an operand it is the comparison operator (`count < 4`), and in `sorted(by: <)` it is the operator itself. After a declaration's name (`func id<T>`, `struct Box<T>`) it begins generic parameters.

After an element's name, a `<` with no space before it begins type arguments: `<List<string> items={xs} />`.

Markup ends at its closing `>` or `/>`: braces that follow it are not a trailing closure.

### 2.4 Elements and components

A name that starts with a lowercase letter, contains `-`, or contains `:` is an element of the page (§4). A name that starts with an uppercase letter, or is dotted, is a component (§5).

Markup is an expression of type `Element`. Parentheses around multi-line markup are ordinary parentheses.

---

## 3. Braces

Braces hold Vertex code. What they give depends on where they are.

### 3.1 As a value

In an attribute that is not a handler, and among children, braces give a value:

| Written | Gives |
| :--- | :--- |
| `{expr}` | the expression's value |
| `{if c { <a/> } else { <b/> }}` | the branch taken; an `if` with no `else` gives nothing when false |
| `{switch x { case .a: <a/> … }}` | the case taken |
| `{cond && <a/>}` | `<a/>` where `cond`, else nothing |
| `{/* comment */}` | nothing |

As a child, a value may be an `Element`, a `String`, an `Int`, a `Double`, an array of them, or an optional of them. `nil` and `Bool` render nothing.

A closure (`{x in …}`) where a value is wanted is refused, except as a component's function child (§5.3). Empty braces are refused.

### 3.2 As a handler

In an `on…` attribute, braces give a handler, a function of the event (§4.3):

```vertex
<button onClick={e in log(e.Type)}>a</button>       // a closure taking the event
<button onClick={n += 1}>b</button>                 // a statement: the event is not used
<button onClick={{ n += 1; save() }}>c</button>     // double braces: a closure of statements
<button onClick={reset}>d</button>                  // a function of no arguments
<button onClick={handle}>e</button>                 // a function of the event
```

Double braces are also how a closure is written as any attribute that takes a function, and with `$0`: `{{ setCount { $0 + 1 } }}`.

---

## 4. Page elements

### 4.1 Names

An element's name is checked: an HTML element, an SVG element, a custom element (`color-picker`), or a namespaced one (`svg:rect`). Anything else is refused, with the nearest name: `unknown element <dvi>; did you mean <div>?`.

### 4.2 Attributes

An element's attributes are checked against what it takes: its own, the global ones (`id`, `class`, `style`, `title`, `hidden`, `tabindex`, `role`, …), any `data-…` or `aria-…`, and any namespaced one. Any attribute is accepted on an SVG element, a custom element, or a namespaced one.

These names are written as in JSX and set as in HTML:

| Written | Sets |
| :--- | :--- |
| `className` | `class` |
| `htmlFor` | `for` |
| `tabIndex`, `readOnly`, `autoFocus`, `maxLength`, `minLength`, `colSpan`, `rowSpan`, `contentEditable`, `spellCheck`, `crossOrigin` | the same name in lowercase |

A value given in braces sets the attribute this way:

| Value | Attribute |
| :--- | :--- |
| `true` | present, empty |
| `false` | absent |
| `Style(…)` | its declarations (§4.4) |
| `[String: String]` | its pairs as declarations, keys sorted (for `style`) |
| anything else | `"\(value)"` |

`{...dict}` with a `[String: String]` sets each pair; `class` and `className` add classes. An attribute written after a spread overrides it.

`key={k}` and `ref={r}` are not attributes of the page (§4.5).

### 4.3 Events

| Attribute | Event | Handler is given |
| :--- | :--- | :--- |
| `onClick` `onDoubleClick` `onContextMenu` `onMouseDown` `onMouseUp` `onMouseMove` `onMouseEnter` `onMouseLeave` `onMouseOver` `onMouseOut` | `click` `dblclick` `contextmenu` `mousedown` … | `dom.MouseEvent` |
| `onPointerDown` `onPointerUp` `onPointerMove` `onPointerEnter` `onPointerLeave` `onPointerCancel` | `pointerdown` … | `dom.PointerEvent` |
| `onWheel` | `wheel` | `dom.WheelEvent` |
| `onKeyDown` `onKeyUp` `onKeyPress` | `keydown` `keyup` `keypress` | `dom.KeyboardEvent` |
| `onInput` `onBeforeInput` | `input` `beforeinput` | `dom.InputEvent` |
| `onSubmit` | `submit` | `dom.SubmitEvent` |
| `onFocus` `onBlur` | `focus` `blur` | `dom.FocusEvent` |
| `onCompositionStart` `onCompositionUpdate` `onCompositionEnd` | `compositionstart` … | `dom.CompositionEvent` |
| `onDrag` `onDragStart` `onDragOver` `onDragEnter` `onDragLeave` `onDragEnd` `onDrop` | `drag` … | `dom.DragEvent` |
| `onLayout` | `layout` | `dom.LayoutEvent` |
| `onChange` `onReset` `onInvalid` `onScroll` `onToggle` `onClose` `onCancel` `onTransitionEnd` `onAnimationStart` `onAnimationEnd` `onAnimationIteration` `onLoad` `onError` `onPlay` `onPause` `onEnded` | the lowercase name | `dom.Event` |

Any of them with `Capture` after it (`onClickCapture`) is the same event. An unknown `on…` name is refused with the nearest one; a handler written as a string is refused.

What every event has: `Type`, `DefaultPrevented`, `Bubbles`, `PreventDefault()`, `StopPropagation()`.

| Event | Adds |
| :--- | :--- |
| `dom.MouseEvent`, `dom.PointerEvent` | `X`, `Y`, `Clicks`, `Button`, `Pressed`, `Shift`, `Control`, `Alt`, `Meta` |
| `dom.WheelEvent` | `X`, `Y`, `DeltaX`, `DeltaY`, `Shift`, `Control`, `Alt`, `Meta` |
| `dom.KeyboardEvent` | `Key`, `Code`, `Shift`, `Control`, `Alt`, `Meta`, `Repeat` |
| `dom.InputEvent` | `Value` (the field's text now), `Data` (what was typed) |

A field is controlled by its state: `value={draft} onInput={e in setDraft(e.Value)}`.

### 4.4 Style

`Style` is typed inline CSS. Its arguments may be written in any order; an `Int` is pixels, a `Double` is written as it is, a `String` as it is, and an unset one is left out.

```vertex
<div style={Style(padding: 8, color: "red", width: w)}>…</div>
// style="color: red; width: 120px; padding: 8px"     (with w = 120)
```

Its properties, and the CSS each sets:

`color`, `backgroundColor` (background-color), `background`, `display`, `position`, `top`, `right`, `bottom`, `left`, `width`, `height`, `minWidth`, `minHeight`, `maxWidth`, `maxHeight`, `margin`, `marginTop`, `marginRight`, `marginBottom`, `marginLeft`, `padding`, `paddingTop`, `paddingRight`, `paddingBottom`, `paddingLeft`, `flex`, `flexDirection`, `alignItems`, `justifyContent`, `gap`, `border`, `borderRadius`, `borderColor`, `fontSize`, `fontWeight`, `lineHeight`, `textAlign`, `textDecoration`, `opacity`, `overflow`, `cursor`, `zIndex`, `transform`, `transition`, `boxShadow` — each `camelCase` name sets its `kebab-case` property.

A property that is not one of these is refused: `extra argument 'colr' in call`. For anything else, and for custom properties, give `style` a dictionary: `style={["--kit-accent": "blue"]}`, or a string: `style="border: 1px solid"`.

### 4.5 `key` and `ref`

`key={k}` names an element among its siblings: the runtime keeps the element that had the key (§8.2).

`ref` gives the element, once it is in the document, to:

| `ref=` | |
| :--- | :--- |
| `{r}`, with `let r = useRef<HTMLElement>()` | `r.current` holds it while it is in the document, `nil` after |
| `{setEl}`, a setter of `useState<HTMLElement?>(nil)` | it is called with it, and with `nil` when it goes |
| `{el in …}` | the closure is called with it, and with `nil` when it goes |

---

## 5. Components

### 5.1 A function

A component is a function returning `Element`. Its parameters are its attributes, by label:

```vertex
func Card(title: string, wide: bool = false, children: Node = nil) -> Element {
    return (
        <section className={wide ? "card wide" : "card"}>
            <h2>{title}</h2>
            {children}
        </section>
    )
}

<Card title="Team" wide><p>Members</p></Card>
```

- An attribute is the argument of the parameter of its label; a bare one is `true`. An attribute no parameter has is refused, with the nearest label: `'Card' has no parameter 'titel'; did you mean 'title'?`. A name with `-`, `:` or `.` is refused: `'data-id' is not a parameter name; 'Icon' is a component`.
- A parameter with no default has to be given: `<Card> needs 'title'`, `<Card> needs children`.
- A parameter that takes a function is given braces as that function (§3.2).
- A component renders on the UI thread: a capitalized top-level function in a `.vsx` file that returns `Element` (or `Node`) is `@MainActor`, and may use `@MainActor` types and functions as they are.
- `{...value}` spreads a struct's fields as the parameters of their names; an attribute written as well wins. Spreading anything but a struct is refused.

### 5.2 A Props struct

A function whose one parameter is unlabeled, or a struct conforming to `Props`, takes its attributes as that struct's memberwise initializer does:

```vertex
struct ButtonProps: Props {
    var label: string
    var kind: string = "plain"
    var children: Node = nil
}

func Button(props: ButtonProps) -> Element {
    return <button className={"btn " + props.kind}>{props.label}{props.children}</button>
}

<Button label="Go" />
<Button {...saved} kind="danger" />
```

An attribute that is no field is refused with the nearest: `'Badge' has no property 'tones'; did you mean 'tone'?`.

### 5.3 Children

What is written between a component's tags is its `children` argument, as the parameter's type takes it:

| `children:` parameter | Given |
| :--- | :--- |
| `Node` | the children as one fragment |
| a function, such as `() -> Node` | a closure making them |
| a function of values, such as `(T) -> Element` | the one closure written as the only child: `<Each items={xs}>{x in <li>{x}</li>}</Each>` |

A function child (`{x in …}`) is accepted only as a component's only child.

### 5.4 Names and type arguments

A component is named as its function is: `Card`, `kit.Card` (through its package), or `ui2.Nav` (a static function of a type). Type arguments are written after the name, or inferred from the attributes:

```vertex
<Picker<Size> value={.large} options={[.small, .large]} name={s in s.rawValue} />
<Picker value={2} options={[1, 2, 3]} name={n in "n\(n)"} />
```

### 5.5 A class

A class inheriting `Component<P, S>`, with an initializer taking no arguments, is a component whose attributes are the fields of `P`:

```vertex
struct ToggleProps {
    var on: string
    var off: string
}

class Toggle: Component<ToggleProps, bool> {
    init() {
        super.init()
        state = false
    }

    override func render() -> Node {
        return <button onClick={{ self.setState(!self.state) }}>{state ? props.on : props.off}</button>
    }
}

<Toggle on="ON" off="off" />
```

`props: P!` and `state: S!` are its props and state; `setState(_:)` sets the state and renders it again. The `ui` runtime also calls `componentDidMount()`, `componentDidUpdate(prevProps:prevState:)` and `componentWillUnmount()`.

### 5.6 `key` and `ref` on a component

`key={k}` names the component among its siblings. `ref` (§4.5) is given the first element the component renders (`ui` only).

### 5.7 Context

```vertex
let Theme = createContext("light")

func Label() -> Element {
    return <span>{use(Theme)}</span>
}

<Theme value="dark"><Label /></Theme>       // <span>dark</span>
<Label />                                    // <span>light</span>
```

`<Ctx value={v}>…</Ctx>` gives `v` to everything rendered inside it, and stops at its end. `use(Ctx)` and `useContext(Ctx)` read the nearest value, or the default outside any.

### 5.8 `Suspense`, `StrictMode`, `Profiler`

`<Suspense fallback={…}>`, `<StrictMode>` and `<Profiler id="…">` render their children, and nothing else.

---

## 6. Hooks

Hooks are called in a component's body, the same ones in the same order every render. A hook called outside a component works once, keeping nothing.

| Hook | |
| :--- | :--- |
| `useState(initial)` → `(value, set)` | `set(v)` sets it, `set { $0 + 1 }` sets it from what it is; either renders the component again. `useState { compute() }` computes the first value once. |
| `useReducer(reducer, initial)` → `(state, dispatch)` | `dispatch(action)` sets the state to `reducer(state, action)` and renders again. |
| `useRef(initial)` → `Ref<T>` | a box kept across renders: `.current` is a `T?`; setting it renders nothing. |
| `useEffect(effect, deps)` | runs `effect` after the render is in the document (its refs given): after the first render, and after each render whose `deps` differ from the last. `deps` `nil` (left out): every render; `[]`: the first only. |
| `useEffect({ …; return { cleanup } }, deps)` | the same, and `cleanup` runs before it runs again and when the component goes. |
| `useLayoutEffect`, `useInsertionEffect` | `useEffect`. |
| `useMemo(compute, deps)` | `compute()`'s value, computed again only when `deps` differ. |
| `useCallback(f, deps)` | `useMemo({ f }, deps)`. |
| `useId()` | an id unique in the program, kept across renders. |
| `useTransition()`, `startTransition(f)`, `useDeferredValue(v)` | renders are synchronous: a transition runs at once, and a deferred value is the value. |
| `createContext(default)`, `use(ctx)`, `useContext(ctx)` | §5.7 |

`deps` is an `[any Equatable]`; `Int`, `String`, `Bool` and `Double` values are compared by value, and any other value differs every render.

Which `useEffect` is meant is told by the closure: one whose body returns a function is the second form.

---

## 7. What markup lowers to

`vsc` checks each piece of markup as the calls it lowers to, against `component`:

| Written | Lowered |
| :--- | :--- |
| `<p className="x">Hi {name}</p>` | `component.Element("p", [component.Attribute.Static("class", "x")], ["Hi ", component.Live { name }])` |
| `<p hidden>` | `component.Attribute.Value("hidden", true)` |
| `title={t}` | `component.Attribute.Live("title", { t })`; a literal: `Attribute.Value` |
| `onClick={…}` | `component.Attribute.On("click", dom.MouseEvent.self, { e in … })` |
| `key={k}`, `ref={r}`, `{...d}` | `component.Attribute.Key(k)`, `.Ref(r)`, `.Spread(d)` |
| `<>…</>` | `component.Fragment([…])` |
| `<Card title="T" />` | `component.Call("main.Card", { Card(title: "T") })` |
| `<Row key={id} />` | `component.Key(id, "main.Row", { Row() })` |
| `<Box ref={r} />` | `component.ForwardRef(r, component.Call("main.Box", { Box() }))` |
| `<Toggle on="a" off="b" />` | `component.MountClass("main.Toggle", Toggle(), ToggleProps(on: "a", off: "b"), { c, p in c._mount(p, { c.render() }) })` |
| `<Theme value="dark">…</Theme>` | `component.Call("main.Theme", { Theme.Provider(value: "dark", children: { … }) })` |

The string in `Call`, `Key` and `MountClass` is the component's identity (§8.2). In a package with styles, each element also gets `component.Attribute.Package(__vssSheet)`, and a call of another package's component is wrapped in `component.Styled(__vssSheet, …)` (`vss.md`).

---

## 8. The `ui` runtime

### 8.1 Mounting

`component.Mount(root, into: document, at: element)` puts what `root` makes into a `dom.Document`; `app.Run(title:width:height:) { <App /> }` does it in a window. The root runs once.

### 8.2 Rendering again

A component renders again when its state is set, or when its parent renders. What it renders is applied to what is already in the document:

- An element keeps its element, and with it its focus and its place; its attributes and handlers are updated, and its children are applied in turn.
- A component keeps its state (its hooks), and renders with the props its parent gives it now.
- Among siblings, a node is matched to the one with its `key`, or, without keys, to the one in its place; only to one of its kind: an element of the same tag, a component of the same identity. Anything else is made anew, and what it replaces is ended: its effects' cleanups run and its refs are given `nil`.
- Keyed siblings are moved, not made again.

Setting a state to a value equal to the one it holds (an `Int`, `String`, `Bool`, `Double`, or an array of `Int`s or `String`s) renders nothing.

### 8.3 Live braces

What markup writes in braces also follows the signals it reads, by itself: an `@Observable` class's properties.

```vertex
@Observable final class Store {
    var Todos: [string] = []
}

func Count(store: Store) -> Element {
    return <p>{store.Todos.count} todos</p>
}
```

When `store.Todos` changes, the text in the braces is made again, and nothing else: `Count` does not render again. A component's body reads untracked: a value it computes from a store outside braces is computed when it renders.

### 8.4 Focus, Bounds, HTML

| | |
| :--- | :--- |
| `Focus(ref)`, `Focus(element)` | moves the page's focus to the element; `Focus(nil)` takes it away |
| `Bounds(of: ref)`, `Bounds(of: element)` | where the element is laid out, or `nil` |
| `node.Html()`, `renderToString(node)` | the node as HTML, as it is now |

`autofocus` focuses its element once it is in the document.

---

## 9. The built-in prelude

Where `ui/component` cannot be found, a `.vsx` file is compiled against `vsc`'s own prelude (`analyzer/vsx_prelude.vs`), which has the names of §1.1 and the events of §4.3. It renders to HTML and runs no page:

- `node.Html()` is the HTML; `node.Fire("click")` calls the first handler for the event found in the tree, in order.
- A component renders again, at once, when its state is set; hooks and context behave as §5–§6 say.
- `Focus` and `Bounds` are not there; a `ref` on a component is not given anything; a class component's `componentDid…`/`componentWillUnmount` are not called.
- Nothing is unmounted: an effect's cleanup runs only before the effect runs again. A component rendered again by its parent starts with new state.
- There is no `<Viewport>`.

The `.vsx` test ladder (`tests/vsx`) runs on it.

---

## 10. Viewports

A viewport is a box of the page that something other than markup draws: a web page, a remote desktop, a virtual machine's screen, a canvas, a terminal. The box is an element like any other -- laid out and styled by the page's CSS, keyed, stamped with its package -- and a **source** fills it.

```vertex
<Viewport source={screen} class="screen" focusable />
```

| Attribute | |
| :--- | :--- |
| `source` | a `viewport.Source`: what draws the box and takes what the user does in it |
| `class` | the element's class (`"viewport"` if left out) |
| `live` | drawn every frame (an animation, a video), not only when the source asks |
| `focusable` | a press in the box gives it the keyboard |

### 10.1 A source

A source is a class inheriting `viewport.Source`. It is `@MainActor`, and so is every subclass. It overrides `Draw`, and what else it takes:

| Method | Called |
| :--- | :--- |
| `Draw(frame: viewport.Frame) -> [uint8]` | for a frame: `frame.Width` × `frame.Height` device pixels, `frame.Scale` device pixels to the CSS pixel, `frame.Time` in seconds. It answers RGBA pixels, premultiplied, top row first; fewer than `Width × Height × 4` leaves the box as the page painted it. |
| `Attached(handle: viewport.Handle)`, `Detached()` | when its box is in the document, and when it is not |
| `PointerDown`, `PointerMove`, `PointerUp` `(e: dom.PointerEvent, x:, y:)`, `PointerLeave()` | the pointer in the box; `x`, `y` in CSS pixels from its corner |
| `Wheel(e: dom.WheelEvent, x:, y:)` | the wheel over the box |
| `KeyDown`, `KeyUp` `(e: dom.KeyboardEvent)` | keys while the box has the keyboard |
| `Text(e: dom.InputEvent)` | text typed while it has the keyboard: `e.Data` |
| `Focus(focused: bool)` | the box took the keyboard, or lost it |

The events then go on to the elements around the box; a source that takes one calls `e.StopPropagation()` (and `e.PreventDefault()`).

`viewport.Drawing { w, h, t in … }` is a source that only draws.

### 10.2 The handle

| | |
| :--- | :--- |
| `Invalidate()` | asks for a frame, from an event or from a task |
| `Live` | whether it is drawn every frame |
| `Cursor` | the pointer's shape over the box, as CSS names it (`"pointer"`, `"text"`, …); `""` for the page's |
| `Focus()` | gives the box the keyboard |
| `Origin`, `Size` | where the box was last drawn, in CSS pixels |

### 10.3 Drawing

The page is painted when it changes; each frame, the host draws the viewports that want one -- live ones, and ones invalidated since -- at their boxes' sizes, copies their pixels into the frame, and paints what the page puts over the boxes (a toolbar, a label) back on top. `ui/app` is the host, and `componenttest`'s `Pixels()` does the same.

### 10.4 A component built on one: `webview.View`

```vertex
import "ui/webview"

func Browser() -> Element {
    let view = useMemo({ webview.WebView() }, [])
    return <webview.View controller={view} url="https://example.com" class="page" />
}
```

`webview.View(url:controller:class:onLoad:)` shows a `webview.WebView`'s page in a viewport: the page laid out at the box's size, clicks, the wheel, keys and typing given to it, its cursor shown over the box, and drawn again when it changes or loads. `url` is navigated to, and again whenever it changes; `controller` is the WebView it shows (one of its own if left out); `onLoad` is given each finished load's `webview.LoadReport`.

