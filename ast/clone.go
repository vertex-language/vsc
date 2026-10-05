package ast

import "reflect"

// Clone copies the tree rooted at n: every node reachable through the
// fields Walk follows is a new node, and everything else -- positions,
// kinds, tokens, the file -- is shared, as it is immutable. The copy has
// the original's spans, so it reads the same source text.
//
// replace, where it is not nil, is asked about each node before it is
// copied: a non-nil answer stands in its place, and is not copied or
// looked into.
func Clone(n Node, replace func(Node) Node) Node {
	if n == nil || isNil(n) {
		return n
	}
	c := &cloner{replace: replace, done: map[any]reflect.Value{}}
	out := c.value(reflect.ValueOf(n))
	if !out.IsValid() {
		return nil
	}
	return out.Interface().(Node)
}

// cloner is one Clone: the replacement asked of each node, and the copy
// made of each node already copied, so that a node reached twice -- a
// trailing closure is in a call's arguments and in its Trailing -- is one
// node in the copy too.
type cloner struct {
	replace func(Node) Node
	done    map[any]reflect.Value
}

func (c *cloner) value(v reflect.Value) reflect.Value {
	switch v.Kind() {
	case reflect.Ptr:
		if v.IsNil() || !v.Type().Implements(nodeType) {
			return v
		}
		key := v.Interface()
		if made, ok := c.done[key]; ok {
			return made
		}
		if c.replace != nil {
			if r := c.replace(v.Interface().(Node)); r != nil {
				return reflect.ValueOf(r)
			}
		}
		elem := v.Elem()
		if elem.Kind() != reflect.Struct {
			return v
		}
		cp := reflect.New(elem.Type())
		cp.Elem().Set(elem)
		c.done[key] = cp
		t := elem.Type()
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if !f.IsExported() || f.Type == spanType || f.Tag.Get("ast") == "-" {
				continue
			}
			field := cp.Elem().Field(i)
			field.Set(c.field(field))
		}
		return cp
	case reflect.Interface:
		if v.IsNil() {
			return v
		}
		inner := c.value(v.Elem())
		out := reflect.New(v.Type()).Elem()
		if inner.IsValid() && inner.Type().AssignableTo(v.Type()) {
			out.Set(inner)
			return out
		}
		return v
	}
	return v
}

// field copies one field's value: a node, an interface holding one,
// or a slice of either.
func (c *cloner) field(v reflect.Value) reflect.Value {
	switch v.Kind() {
	case reflect.Slice:
		if v.IsNil() {
			return v
		}
		out := reflect.MakeSlice(v.Type(), v.Len(), v.Len())
		for i := 0; i < v.Len(); i++ {
			el := c.field(v.Index(i))
			if el.IsValid() && el.Type().AssignableTo(v.Type().Elem()) {
				out.Index(i).Set(el)
			} else {
				out.Index(i).Set(v.Index(i))
			}
		}
		return out
	case reflect.Ptr, reflect.Interface:
		out := c.value(v)
		if !out.IsValid() {
			return reflect.Zero(v.Type())
		}
		if !out.Type().AssignableTo(v.Type()) {
			return v
		}
		if out.Type() != v.Type() {
			conv := reflect.New(v.Type()).Elem()
			conv.Set(out)
			return conv
		}
		return out
	}
	return v
}
