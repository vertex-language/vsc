package analyzer

import "strings"

// The HTML standard's elements and attributes, as markup in a .vsx file
// checks them: `<dvi>` and `<a hreff>` are errors with a suggestion. The
// engine's own tables (web/dom) will replace these once vsc can read
// them; see proposed_vsx.md §13, question 10.

// htmlElements are the HTML elements, each with the attributes it takes
// beyond the global ones.
var htmlElements = map[string][]string{
	"a":    {"href", "target", "download", "ping", "rel", "hreflang", "type", "referrerpolicy"},
	"abbr": nil, "address": nil,
	"area":    {"alt", "coords", "shape", "href", "target", "download", "ping", "rel", "referrerpolicy"},
	"article": nil, "aside": nil,
	"audio": {"src", "crossorigin", "preload", "autoplay", "loop", "muted", "controls"},
	"b":     nil, "bdi": nil, "bdo": nil,
	"blockquote": {"cite"},
	"body":       nil, "br": nil,
	"button":  {"disabled", "form", "formaction", "formenctype", "formmethod", "formnovalidate", "formtarget", "name", "popovertarget", "popovertargetaction", "type", "value"},
	"canvas":  {"width", "height"},
	"caption": nil, "cite": nil, "code": nil,
	"col":      {"span"},
	"colgroup": {"span"},
	"data":     {"value"},
	"datalist": nil, "dd": nil,
	"del":     {"cite", "datetime"},
	"details": {"open", "name"},
	"dfn":     nil,
	"dialog":  {"open"},
	"div":     nil, "dl": nil, "dt": nil, "em": nil,
	"embed":      {"src", "type", "width", "height"},
	"fieldset":   {"disabled", "form", "name"},
	"figcaption": nil, "figure": nil, "footer": nil,
	"form": {"accept-charset", "action", "autocomplete", "enctype", "method", "name", "novalidate", "target", "rel"},
	"h1":   nil, "h2": nil, "h3": nil, "h4": nil, "h5": nil, "h6": nil,
	"head": nil, "header": nil, "hgroup": nil, "hr": nil,
	"html":   nil,
	"i":      nil,
	"iframe": {"src", "srcdoc", "name", "sandbox", "allow", "allowfullscreen", "width", "height", "referrerpolicy", "loading"},
	"img":    {"alt", "src", "srcset", "sizes", "crossorigin", "usemap", "ismap", "width", "height", "referrerpolicy", "decoding", "loading", "fetchpriority"},
	"input": {"accept", "alt", "autocomplete", "checked", "dirname", "disabled", "form", "formaction", "formenctype", "formmethod",
		"formnovalidate", "formtarget", "height", "list", "max", "maxlength", "min", "minlength", "multiple", "name", "pattern",
		"placeholder", "popovertarget", "popovertargetaction", "readonly", "required", "size", "src", "step", "type", "value", "width"},
	"ins":    {"cite", "datetime"},
	"kbd":    nil,
	"label":  {"for"},
	"legend": nil,
	"li":     {"value"},
	"link":   {"href", "crossorigin", "rel", "media", "integrity", "hreflang", "type", "referrerpolicy", "sizes", "as", "disabled"},
	"main":   nil,
	"map":    {"name"},
	"mark":   nil, "menu": nil,
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
	"pre":      nil,
	"progress": {"value", "max"},
	"q":        {"cite"},
	"rp":       nil, "rt": nil, "ruby": nil, "s": nil, "samp": nil,
	"script": {"src", "type", "nomodule", "async", "defer", "crossorigin", "integrity", "referrerpolicy"},
	"search": nil, "section": nil,
	"select": {"autocomplete", "disabled", "form", "multiple", "name", "required", "size"},
	"slot":   {"name"},
	"small":  nil,
	"source": {"type", "media", "src", "srcset", "sizes", "width", "height"},
	"span":   nil, "strong": nil,
	"style": {"media"},
	"sub":   nil, "summary": nil, "sup": nil,
	"table": nil, "tbody": nil,
	"td":       {"colspan", "rowspan", "headers"},
	"template": nil,
	"textarea": {"autocomplete", "cols", "dirname", "disabled", "form", "maxlength", "minlength", "name", "placeholder", "readonly", "required", "rows", "wrap", "value"},
	"tfoot":    nil,
	"th":       {"colspan", "rowspan", "headers", "scope", "abbr"},
	"thead":    nil,
	"time":     {"datetime"},
	"title":    nil, "tr": nil,
	"track": {"default", "kind", "label", "src", "srclang"},
	"u":     nil, "ul": nil, "var": nil,
	"video": {"src", "crossorigin", "poster", "preload", "autoplay", "playsinline", "loop", "muted", "controls", "width", "height"},
	"wbr":   nil,
	// Inline SVG, which the engine draws (web/svg). Its attributes are
	// not checked: SVG's are many and case-sensitive.
	"svg": nil, "path": nil, "g": nil, "circle": nil, "rect": nil, "line": nil,
	"polyline": nil, "polygon": nil, "ellipse": nil,
}

// svgElements take any attribute.
var svgElements = map[string]bool{
	"svg": true, "path": true, "g": true, "circle": true, "rect": true, "line": true,
	"polyline": true, "polygon": true, "ellipse": true,
}

// htmlGlobalAttributes are the attributes every element takes.
var htmlGlobalAttributes = map[string]bool{
	"accesskey": true, "autocapitalize": true, "autofocus": true, "class": true, "contenteditable": true,
	"dir": true, "draggable": true, "enterkeyhint": true, "hidden": true, "id": true, "inert": true,
	"inputmode": true, "is": true, "itemid": true, "itemprop": true, "itemref": true, "itemscope": true,
	"itemtype": true, "lang": true, "nonce": true, "popover": true, "slot": true, "spellcheck": true,
	"style": true, "tabindex": true, "title": true, "translate": true, "role": true, "part": true,
	// The engine's native additions (proposed_vsx.md §8.1).
	"titlebar": true, "material": true, "dragRegion": true,
}

// htmlEvents are the event attributes markup takes, by the name written,
// with the DOM event's name and the type of event its handler is given.
var htmlEvents = map[string]struct{ event, typ string }{
	"onClick":         {"click", "MouseEvent"},
	"onDoubleClick":   {"dblclick", "MouseEvent"},
	"onContextMenu":   {"contextmenu", "MouseEvent"},
	"onMouseDown":     {"mousedown", "MouseEvent"},
	"onMouseUp":       {"mouseup", "MouseEvent"},
	"onMouseMove":     {"mousemove", "MouseEvent"},
	"onMouseEnter":    {"mouseenter", "MouseEvent"},
	"onMouseLeave":    {"mouseleave", "MouseEvent"},
	"onPointerDown":   {"pointerdown", "PointerEvent"},
	"onPointerUp":     {"pointerup", "PointerEvent"},
	"onPointerMove":   {"pointermove", "PointerEvent"},
	"onWheel":         {"wheel", "WheelEvent"},
	"onKeyDown":       {"keydown", "KeyboardEvent"},
	"onKeyUp":         {"keyup", "KeyboardEvent"},
	"onInput":         {"input", "InputEvent"},
	"onChange":        {"change", "Event"},
	"onSubmit":        {"submit", "SubmitEvent"},
	"onReset":         {"reset", "Event"},
	"onFocus":         {"focus", "FocusEvent"},
	"onBlur":          {"blur", "FocusEvent"},
	"onScroll":        {"scroll", "Event"},
	"onToggle":        {"toggle", "Event"},
	"onComposition":   {"compositionupdate", "CompositionEvent"},
	"onDragStart":     {"dragstart", "DragEvent"},
	"onDragOver":      {"dragover", "DragEvent"},
	"onDrop":          {"drop", "DragEvent"},
	"onLayout":        {"layout", "LayoutEvent"},
	"onTransitionEnd": {"transitionend", "Event"},
}

// htmlAttributeKnown reports whether an element takes an attribute.
func htmlAttributeKnown(tag, name string) bool {
	if svgElements[tag] || htmlGlobalAttributes[name] ||
		strings.HasPrefix(name, "data-") || strings.HasPrefix(name, "aria-") {
		return true
	}
	for _, a := range htmlElements[tag] {
		if a == name {
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
