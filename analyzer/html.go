package analyzer

import "strings"

// The HTML & SVG elements and attributes, supporting both standard HTML
// attribute names and TSX/JSX camelCase attribute aliases (className,
// htmlFor, tabIndex, maxLength, readOnly, etc.).

// htmlElements are the HTML elements, each with the attributes it takes
// beyond the global ones.
var htmlElements = map[string][]string{
	"a":       {"href", "target", "download", "ping", "rel", "hreflang", "type", "referrerpolicy"},
	"abbr":    nil,
	"address": nil,
	"area":    {"alt", "coords", "shape", "href", "target", "download", "ping", "rel", "referrerpolicy"},
	"article": nil,
	"aside":   nil,
	"audio":   {"src", "crossorigin", "preload", "autoplay", "loop", "muted", "controls"},
	"b":       nil, "bdi": nil, "bdo": nil,
	"blockquote": {"cite"},
	"body":       nil, "br": nil,
	"button":   {"disabled", "form", "formaction", "formenctype", "formmethod", "formnovalidate", "formtarget", "name", "popovertarget", "popovertargetaction", "type", "value"},
	"canvas":   {"width", "height"},
	"caption":  nil, "cite": nil, "code": nil,
	"col":      {"span"},
	"colgroup": {"span"},
	"data":     {"value"},
	"datalist": nil, "dd": nil,
	"del":      {"cite", "datetime"},
	"details":  {"open", "name"},
	"dfn":      nil,
	"dialog":   {"open"},
	"div":      nil, "dl": nil, "dt": nil, "em": nil,
	"embed":      {"src", "type", "width", "height"},
	"fieldset":   {"disabled", "form", "name"},
	"figcaption": nil, "figure": nil, "footer": nil,
	"form":       {"accept-charset", "action", "autocomplete", "enctype", "method", "name", "novalidate", "target", "rel"},
	"h1":         nil, "h2": nil, "h3": nil, "h4": nil, "h5": nil, "h6": nil,
	"head":       nil, "header": nil, "hgroup": nil, "hr": nil,
	"html":       {"xmlns"},
	"i":          nil,
	"iframe":     {"src", "srcdoc", "name", "sandbox", "allow", "allowfullscreen", "width", "height", "referrerpolicy", "loading"},
	"img":        {"alt", "src", "srcset", "sizes", "crossorigin", "usemap", "ismap", "width", "height", "referrerpolicy", "decoding", "loading", "fetchpriority"},
	"input": {"accept", "alt", "autocomplete", "checked", "dirname", "disabled", "form", "formaction", "formenctype", "formmethod",
		"formnovalidate", "formtarget", "height", "list", "max", "maxlength", "min", "minlength", "multiple", "name", "pattern",
		"placeholder", "popovertarget", "popovertargetaction", "readonly", "required", "size", "src", "step", "type", "value", "width"},
	"ins":      {"cite", "datetime"},
	"kbd":      nil,
	"label":    {"for"},
	"legend":   nil,
	"li":       {"value"},
	"link":     {"href", "crossorigin", "rel", "media", "integrity", "hreflang", "type", "referrerpolicy", "sizes", "as", "disabled"},
	"main":     nil,
	"map":      {"name"},
	"mark":     nil, "menu": nil,
	"meta":     {"name", "http-equiv", "content", "charset", "media"},
	"meter":    {"value", "min", "max", "low", "high", "optimum"},
	"nav":      nil,
	"noscript": nil,
	"object":   {"data", "type", "name", "form", "width", "height"},
	"ol":       {"reversed", "start", "type"},
	"optgroup": {"disabled", "label"},
	"option":   {"disabled", "label", "selected", "value"},
	"output":   {"for", "form", "name"},
	"p":        nil,
	"picture":  nil,
	"portal":   {"src", "referrerpolicy"},
	"pre":      nil,
	"progress": {"value", "max"},
	"q":        {"cite"},
	"rp":       nil, "rt": nil, "ruby": nil, "s": nil, "samp": nil,
	"script":   {"src", "type", "nomodule", "async", "defer", "crossorigin", "integrity", "referrerpolicy"},
	"search":   nil, "section": nil,
	"select":   {"autocomplete", "disabled", "form", "multiple", "name", "required", "size", "value"},
	"slot":     {"name"},
	"small":    nil,
	"source":   {"type", "media", "src", "srcset", "sizes", "width", "height"},
	"span":     nil, "strong": nil,
	"style":    {"media"},
	"sub":      nil, "summary": nil, "sup": nil,
	"table":    nil, "tbody": nil,
	"td":       {"colspan", "rowspan", "headers"},
	"template": nil,
	"textarea": {"autocomplete", "cols", "dirname", "disabled", "form", "maxlength", "minlength", "name", "placeholder", "readonly", "required", "rows", "wrap", "value"},
	"tfoot":    nil,
	"th":       {"colspan", "rowspan", "headers", "scope", "abbr"},
	"thead":    nil,
	"time":     {"datetime"},
	"title":    nil, "tr": nil,
	"track":    {"default", "kind", "label", "src", "srclang"},
	"u":        nil, "ul": nil, "var": nil,
	"video":    {"src", "crossorigin", "poster", "preload", "autoplay", "playsinline", "loop", "muted", "controls", "width", "height"},
	"wbr":      nil,
	// Inline SVG elements.
	"svg": nil, "path": nil, "g": nil, "circle": nil, "rect": nil, "line": nil,
	"polyline": nil, "polygon": nil, "ellipse": nil, "defs": nil, "linearGradient": nil,
	"radialGradient": nil, "stop": nil, "text": nil, "tspan": nil, "use": nil,
	"symbol": nil, "clipPath": nil, "mask": nil, "pattern": nil, "filter": nil,
	"feGaussianBlur": nil, "feOffset": nil, "feBlend": nil, "feColorMatrix": nil,
	"foreignObject": nil, "marker": nil, "image": nil, "desc": nil,
}

// svgElements take any attribute.
var svgElements = map[string]bool{
	"svg": true, "path": true, "g": true, "circle": true, "rect": true, "line": true,
	"polyline": true, "polygon": true, "ellipse": true, "defs": true, "linearGradient": true,
	"radialGradient": true, "stop": true, "text": true, "tspan": true, "use": true,
	"symbol": true, "clipPath": true, "mask": true, "pattern": true, "filter": true,
	"feGaussianBlur": true, "feOffset": true, "feBlend": true, "feColorMatrix": true,
	"foreignObject": true, "marker": true, "image": true, "desc": true,
}

// tsxAttributeAliases maps JSX/TSX camelCase attribute names to their
// canonical HTML attribute names.
var tsxAttributeAliases = map[string]string{
	"className":           "class",
	"htmlFor":             "for",
	"tabIndex":            "tabindex",
	"readOnly":            "readonly",
	"autoFocus":           "autofocus",
	"maxLength":           "maxlength",
	"minLength":           "minlength",
	"colSpan":             "colspan",
	"rowSpan":             "rowspan",
	"contentEditable":     "contenteditable",
	"spellCheck":          "spellcheck",
	"crossOrigin":         "crossorigin",
	"srcSet":              "srcset",
	"autoComplete":        "autocomplete",
	"autoPlay":            "autoplay",
	"playsInline":         "playsinline",
	"noValidate":          "novalidate",
	"formAction":          "formaction",
	"formEncType":         "formenctype",
	"formMethod":          "formmethod",
	"formNoValidate":      "formnovalidate",
	"formTarget":          "formtarget",
	"acceptCharset":       "accept-charset",
	"httpEquiv":           "http-equiv",
	"fetchPriority":       "fetchpriority",
	"referrerPolicy":      "referrerpolicy",
	"allowFullScreen":     "allowfullscreen",
	"useMap":              "usemap",
	"isMap":               "ismap",
	"dateTime":            "datetime",
	"srcDoc":              "srcdoc",
	"srcLang":             "srclang",
	"enterKeyHint":        "enterkeyhint",
	"inputMode":           "inputmode",
	"accessKey":           "accesskey",
	"autoCapitalize":      "autocapitalize",
	"itemId":              "itemid",
	"itemProp":            "itemprop",
	"itemRef":             "itemref",
	"itemScope":           "itemscope",
	"itemType":            "itemtype",
	"popoverTarget":       "popovertarget",
	"popoverTargetAction": "popovertargetaction",
	"defaultValue":        "value",
	"defaultChecked":      "checked",
}

func canonicalHTMLAttribute(name string) string {
	if c, ok := tsxAttributeAliases[name]; ok {
		return c
	}
	return name
}

// htmlGlobalAttributes are the attributes every element takes.
var htmlGlobalAttributes = map[string]bool{
	"accesskey": true, "autocapitalize": true, "autofocus": true, "class": true, "contenteditable": true,
	"dir": true, "draggable": true, "enterkeyhint": true, "hidden": true, "id": true, "inert": true,
	"inputmode": true, "is": true, "itemid": true, "itemprop": true, "itemref": true, "itemscope": true,
	"itemtype": true, "lang": true, "nonce": true, "popover": true, "slot": true, "spellcheck": true,
	"style": true, "tabindex": true, "title": true, "translate": true, "role": true, "part": true,
	"exportparts": true, "virtualkeyboardpolicy": true, " suppressContentEditableWarning": true,
	// Engine native window/layout attributes.
	"titlebar": true, "material": true, "dragRegion": true,
}

// htmlEvents are the event attributes markup takes, by the name written,
// with the DOM event's name and the type of event its handler is given. Only
// what the engine dispatches is here: one it never fires would be a handler
// that silently never runs, so onLoad, onError, onPlay, onPause, onEnded,
// onKeyPress and onLayout join when it does (vsx_ui_web_TODO.md).
var htmlEvents = map[string]struct{ event, typ string }{
	"onClick":              {"click", "MouseEvent"},
	"onDoubleClick":        {"dblclick", "MouseEvent"},
	"onContextMenu":        {"contextmenu", "MouseEvent"},
	"onMouseDown":          {"mousedown", "MouseEvent"},
	"onMouseUp":            {"mouseup", "MouseEvent"},
	"onMouseMove":          {"mousemove", "MouseEvent"},
	"onMouseEnter":         {"mouseenter", "MouseEvent"},
	"onMouseLeave":         {"mouseleave", "MouseEvent"},
	"onMouseOver":          {"mouseover", "MouseEvent"},
	"onMouseOut":           {"mouseout", "MouseEvent"},
	"onPointerDown":        {"pointerdown", "PointerEvent"},
	"onPointerUp":          {"pointerup", "PointerEvent"},
	"onPointerMove":        {"pointermove", "PointerEvent"},
	"onPointerEnter":       {"pointerenter", "PointerEvent"},
	"onPointerLeave":       {"pointerleave", "PointerEvent"},
	"onPointerCancel":      {"pointercancel", "PointerEvent"},
	"onWheel":              {"wheel", "WheelEvent"},
	"onKeyDown":            {"keydown", "KeyboardEvent"},
	"onKeyUp":              {"keyup", "KeyboardEvent"},
	"onInput":              {"input", "InputEvent"},
	"onBeforeInput":        {"beforeinput", "InputEvent"},
	"onChange":             {"change", "Event"},
	"onSubmit":             {"submit", "SubmitEvent"},
	"onReset":              {"reset", "Event"},
	"onInvalid":            {"invalid", "Event"},
	"onFocus":              {"focus", "FocusEvent"},
	"onBlur":               {"blur", "FocusEvent"},
	"onScroll":             {"scroll", "Event"},
	"onToggle":             {"toggle", "ToggleEvent"},
	"onBeforeToggle":       {"beforetoggle", "ToggleEvent"},
	"onCommand":            {"command", "Event"},
	"onClose":              {"close", "Event"},
	"onCancel":             {"cancel", "Event"},
	"onComposition":        {"compositionupdate", "CompositionEvent"},
	"onCompositionStart":   {"compositionstart", "CompositionEvent"},
	"onCompositionUpdate":  {"compositionupdate", "CompositionEvent"},
	"onCompositionEnd":     {"compositionend", "CompositionEvent"},
	"onDrag":               {"drag", "DragEvent"},
	"onDragStart":          {"dragstart", "DragEvent"},
	"onDragOver":           {"dragover", "DragEvent"},
	"onDragEnter":          {"dragenter", "DragEvent"},
	"onDragLeave":          {"dragleave", "DragEvent"},
	"onDragEnd":            {"dragend", "DragEvent"},
	"onDrop":               {"drop", "DragEvent"},
	"onTransitionEnd":      {"transitionend", "TransitionEvent"},
	"onTransitionRun":      {"transitionrun", "TransitionEvent"},
	"onTransitionStart":    {"transitionstart", "TransitionEvent"},
	"onTransitionCancel":   {"transitioncancel", "TransitionEvent"},
	"onAnimationStart":     {"animationstart", "AnimationEvent"},
	"onAnimationCancel":    {"animationcancel", "AnimationEvent"},
	"onAnimationEnd":       {"animationend", "AnimationEvent"},
	"onAnimationIteration": {"animationiteration", "AnimationEvent"},
}

func lookupHTMLEvent(name string) (struct{ event, typ string }, bool) {
	if ev, ok := htmlEvents[name]; ok {
		return ev, true
	}
	if strings.HasSuffix(name, "Capture") {
		base := strings.TrimSuffix(name, "Capture")
		if ev, ok := htmlEvents[base]; ok {
			return ev, true
		}
	}
	return struct{ event, typ string }{}, false
}

func isKnownIntrinsicElement(tag string) bool {
	if _, ok := htmlElements[tag]; ok {
		return true
	}
	// Custom web components (<my-element>) or XML-namespaced tags (<svg:rect>).
	return strings.Contains(tag, "-") || strings.Contains(tag, ":")
}

// htmlAttributeKnown reports whether an element takes an attribute.
func htmlAttributeKnown(tag, name string) bool {
	canon := canonicalHTMLAttribute(name)
	if svgElements[tag] || strings.Contains(tag, "-") || strings.Contains(tag, ":") ||
		htmlGlobalAttributes[canon] || htmlGlobalAttributes[name] ||
		strings.HasPrefix(name, "data-") || strings.HasPrefix(name, "aria-") ||
		strings.Contains(name, ":") {
		return true
	}
	for _, a := range htmlElements[tag] {
		if a == canon || a == name {
			return true
		}
	}
	return false
}

// htmlAttributesOf is every attribute name an element takes, for a
// suggestion.
func htmlAttributesOf(tag string) []string {
	var out []string
	for a := range htmlGlobalAttributes {
		out = append(out, a)
	}
	for alias := range tsxAttributeAliases {
		out = append(out, alias)
	}
	out = append(out, htmlElements[tag]...)
	for e := range htmlEvents {
		out = append(out, e)
	}
	return out
}

// htmlElementNames is every element name, for a suggestion.
func htmlElementNames() []string {
	var out []string
	for e := range htmlElements {
		out = append(out, e)
	}
	return out
}
