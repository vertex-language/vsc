// Describing a value whose type is known only by its metadata: what
// print writes, and what a string interpolation's holes become.
#include "vertex/abi.h"
#include "vertex/platform.h"
#include "text.h"
#include "dtoa.h"

extern "C" {
extern const vertex::FullMetadata vertex_metadata_Bool, vertex_metadata_Int8,
    vertex_metadata_UInt8, vertex_metadata_Int16, vertex_metadata_UInt16,
    vertex_metadata_Int32, vertex_metadata_UInt32, vertex_metadata_Float,
    vertex_metadata_Int, vertex_metadata_UInt, vertex_metadata_Int64,
    vertex_metadata_UInt64, vertex_metadata_Double, vertex_metadata_String,
    vertex_metadata_Any;
}

namespace vertex {

inline bool is(const Metadata* type, const FullMetadata& record) {
  return static_cast<const void*>(type) == static_cast<const void*>(&record.metadata);
}

inline const void* anyContents(const AnyExistential* any) {
  const ValueWitnessTable* vw = witnesses(any->type);
  if (vw->flags & vwIsNonInline)
    return reinterpret_cast<const u8*>(any->buffer[0]) + boxValueOffset(vw->flags);
  return any->buffer;
}

inline void describeString(Text& t, const String& s) {
  u8 scratch[16];
  StringBytes b = bytesOf(s, scratch);
  textAppend(t, b.bytes, b.count);
}

void describe(Text& t, const void* value, const Metadata* type);
void debugDescribe(Text& t, const void* value, const Metadata* type);
void typeName(Text& t, const Metadata* type);

// describeConforming writes the description of a value whose type is
// CustomStringConvertible, and reports whether it is. The table's first
// row after its descriptor is the description getter.
inline bool describeConforming(Text& t, const void* value, const Metadata* type) {
  const void* const* table = vertex_conformance(type, &vertex_protocol_CustomStringConvertible);
  if (table == nullptr)
    return false;
  auto* getter = static_cast<void (*)()>(const_cast<void*>(table[1]));
  String s = vertex_witness_call(getter, value, type, table);
  describeString(t, s);
  vertex_string_release(s.object);
  return true;
}

// debugDescribeConforming is describeConforming for
// CustomDebugStringConvertible: the debugDescription, which Swift's debug
// description prefers to the description.
inline bool debugDescribeConforming(Text& t, const void* value, const Metadata* type) {
  const void* const* table = vertex_conformance(type, &vertex_protocol_CustomDebugStringConvertible);
  if (table == nullptr)
    return false;
  auto* getter = static_cast<void (*)()>(const_cast<void*>(table[1]));
  String s = vertex_witness_call(getter, value, type, table);
  describeString(t, s);
  vertex_string_release(s.object);
  return true;
}

// appendScalar writes one scalar as UTF-8.
inline void appendScalar(Text& t, u32 c) {
  u8 b[4];
  usize n;
  if (c < 0x80) {
    b[0] = static_cast<u8>(c);
    n = 1;
  } else if (c < 0x800) {
    b[0] = static_cast<u8>(0xC0 | (c >> 6));
    b[1] = static_cast<u8>(0x80 | (c & 0x3F));
    n = 2;
  } else if (c < 0x10000) {
    b[0] = static_cast<u8>(0xE0 | (c >> 12));
    b[1] = static_cast<u8>(0x80 | ((c >> 6) & 0x3F));
    b[2] = static_cast<u8>(0x80 | (c & 0x3F));
    n = 3;
  } else {
    b[0] = static_cast<u8>(0xF0 | (c >> 18));
    b[1] = static_cast<u8>(0x80 | ((c >> 12) & 0x3F));
    b[2] = static_cast<u8>(0x80 | ((c >> 6) & 0x3F));
    b[3] = static_cast<u8>(0x80 | (c & 0x3F));
    n = 4;
  }
  textAppend(t, b, n);
}

// appendEscapedScalar writes \u{...}, with the digits padded to two,
// four or eight as Swift pads them.
inline void appendEscapedScalar(Text& t, u32 c) {
  static const char hex[] = "0123456789ABCDEF";
  usize digits = c <= 0xFF ? 2 : c <= 0xFFFF ? 4 : 8;
  textString(t, "\\u{");
  for (usize i = digits; i > 0; i--)
    textByte(t, static_cast<u8>(hex[(c >> (4 * (i - 1))) & 0xF]));
  textByte(t, '}');
}

// specialEscape is the short escape Swift writes for a scalar that has
// one, or null.
inline const char* specialEscape(u32 c) {
  switch (c) {
  case 0: return "\\0";
  case '\t': return "\\t";
  case '\n': return "\\n";
  case '\r': return "\\r";
  case '"': return "\\\"";
  case '\'': return "\\'";
  case '\\': return "\\\\";
  }
  return nullptr;
}

// debugString writes a String the way its debugDescription does:
// quoted, with the ASCII controls, quotes and backslash escaped, and any
// scalar escaped that would otherwise fuse into one Character with the
// quote or with an escape before it.
inline void debugString(Text& t, const String& s) {
  u8 scratch[16];
  StringBytes b = bytesOf(s, scratch);
  GraphemeState fresh{0, 0, 0};
  textByte(t, '"');
  // Where each scalar written raw began, so that the end can take the
  // trailing ones back.
  struct Raw {
    usize at;
    u32   scalar;
  };
  auto* raws = static_cast<Raw*>(vertex_pal_alloc(sizeof(Raw) * (b.count + 1), 8));
  if (raws == nullptr)
    vertex_pal_abort();
  usize rawCount = 0;
  u32 last = '"';
  bool wantBreak = true;
  bool trailingRaw = false;
  for (usize i = 0; i < b.count;) {
    u32 c = decodeScalar(b.bytes, b.count, &i);
    if (const char* e = specialEscape(c)) {
      textString(t, e);
      last = static_cast<u8>(e[1]);
      wantBreak = true;
      trailingRaw = false;
    } else if (c < 0x20 || c == 0x7F ||
               (wantBreak && !breaksBetween(graphemeProperty(last), graphemeProperty(c), fresh))) {
      appendEscapedScalar(t, c);
      last = '}';
      wantBreak = true;
      trailingRaw = false;
    } else {
      if (!trailingRaw)
        rawCount = 0;
      raws[rawCount++] = {t.count, c};
      appendScalar(t, c);
      last = c;
      wantBreak = false;
      trailingRaw = true;
    }
  }
  // Nor may the last scalars fuse with the closing quote. Escaping one
  // puts a backslash after the one before it, which may fuse with that in
  // turn, so this walks back until something breaks.
  if (trailingRaw) {
    u32 next = '"';
    usize keep = rawCount;
    while (keep > 0 && !breaksBetween(graphemeProperty(raws[keep - 1].scalar), graphemeProperty(next), fresh)) {
      keep--;
      next = '\\';
    }
    if (keep < rawCount) {
      t.count = raws[keep].at;
      for (usize k = keep; k < rawCount; k++)
        appendEscapedScalar(t, raws[k].scalar);
    }
  }
  vertex_pal_free(raws, 0, 8);
  textByte(t, '"');
}

// qualifiedName writes a nominal type's module and name: `main.Point`.
inline void qualifiedName(Text& t, const NominalTypeDescriptor* d) {
  if (d == nullptr) {
    textString(t, "<unknown>");
    return;
  }
  if (auto* module = static_cast<const ModuleDescriptor*>(relative(&d->parent))) {
    if (const char* name = relativeString(&module->name)) {
      textString(t, name);
      textByte(t, '.');
    }
  }
  if (const char* name = relativeString(&d->name))
    textString(t, name);
}

// describeStruct writes a struct as Swift's reflection does: its name,
// then each field's name and its value's debug description. Where it is
// itself a field, the name is qualified by its module. A struct whose
// descriptor does not describe its fields is its name and ().
inline void describeStruct(Text& t, const void* value, const StructMetadata* type, bool qualified) {
  const NominalTypeDescriptor* d = type->description;
  if (d == nullptr) {
    textString(t, "<unknown>");
    return;
  }
  if (qualified) {
    if (auto* module = static_cast<const ModuleDescriptor*>(relative(&d->parent))) {
      if (const char* name = relativeString(&module->name)) {
        textString(t, name);
        textByte(t, '.');
      }
    }
  }
  if (const char* name = relativeString(&d->name))
    textString(t, name);
  textByte(t, '(');
  if (auto* fields = static_cast<const FieldDescriptor*>(relative(&d->fields))) {
    auto* records = reinterpret_cast<const FieldRecord*>(fields + 1);
    auto* offsets = reinterpret_cast<const u32*>(reinterpret_cast<const u8*>(type) + structFieldOffsets);
    for (u64 i = 0; i < fields->count; i++) {
      if (i > 0)
        textString(t, ", ");
      textString(t, records[i].name);
      textString(t, ": ");
      debugDescribe(t, static_cast<const u8*>(value) + offsets[i], &records[i].type->metadata);
    }
  }
  textByte(t, ')');
}

// describeTupleElements writes a tuple's elements, each after its label
// where it has one, without the parentheses around them.
inline void describeTupleElements(Text& t, const void* value, const TupleMetadata* type) {
  const char* label = type->labels;
  for (usize i = 0; i < type->numElements; i++) {
    if (i > 0)
      textString(t, ", ");
    if (label != nullptr) {
      const char* end = label;
      while (*end != '\0' && *end != ' ')
        end++;
      if (end != label) {
        while (label != end)
          textByte(t, *label++);
        textString(t, ": ");
      }
      label = *end == ' ' ? end + 1 : end;
    }
    debugDescribe(t, static_cast<const u8*>(value) + type->elements[i].offset, type->elements[i].type);
  }
}

// describeTuple writes a tuple as Swift does: its elements in parentheses.
inline void describeTuple(Text& t, const void* value, const TupleMetadata* type) {
  textByte(t, '(');
  describeTupleElements(t, value, type);
  textByte(t, ')');
}

// describeEnum writes an enum as Swift's reflection does: its case, and
// what the case carries in parentheses. Where it is itself a field or an
// element the case is qualified by the type: `main.Suit.spades`.
//
// The descriptor's fields are its cases in tag order, and its last word
// is where the tag starts (see vsc/lower's enumDescriptor). The tag is
// the bytes from there to the end of the value.
inline void describeEnum(Text& t, const void* value, const StructMetadata* type, bool qualified) {
  const NominalTypeDescriptor* d = type->description;
  auto* cases = d != nullptr ? static_cast<const FieldDescriptor*>(relative(&d->fields)) : nullptr;
  if (cases == nullptr) {
    textString(t, "<unknown>");
    return;
  }
  usize offset = d->fieldOffsetVectorOffset;
  usize size = witnesses(type)->size;
  u64 tag = 0;
  for (usize i = offset; i < size && i < offset + 8; i++)
    tag |= static_cast<u64>(static_cast<const u8*>(value)[i]) << (8 * (i - offset));
  if (tag >= cases->count) {
    textString(t, "<unknown>");
    return;
  }
  const FieldRecord& c = reinterpret_cast<const FieldRecord*>(cases + 1)[tag];
  if (qualified) {
    qualifiedName(t, d);
    textByte(t, '.');
  }
  textString(t, c.name);
  // The record's low bit says the case is indirect: the value is a
  // box, and the payload is inside it past the header.
  auto raw = reinterpret_cast<usize>(c.type);
  auto* payloadType = reinterpret_cast<const FullMetadata*>(raw & ~static_cast<usize>(1));
  if (payloadType != nullptr) {
    const void* payload = value;
    if (raw & 1)
      payload = reinterpret_cast<const u8*>(*static_cast<const HeapObject* const*>(value)) + sizeof(HeapObject);
    textByte(t, '(');
    // Several values a case carries are its tuple's elements, written
    // as the arguments they were.
    if (payloadType->metadata.kind == kindTuple) {
      auto* tuple = reinterpret_cast<const TupleMetadata*>(&payloadType->metadata);
      if (tuple->numElements > 1) {
        describeTupleElements(t, payload, tuple);
        textByte(t, ')');
        return;
      }
    }
    debugDescribe(t, payload, &payloadType->metadata);
    textByte(t, ')');
  }
}

// debugDescribe is describe as a field's value is written: a String
// quoted and escaped, a nested struct qualified by its module.
inline void debugDescribe(Text& t, const void* value, const Metadata* type) {
  // What an existential holds is described as that value would be here.
  if (type->kind == kindExistential) {
    auto* any = static_cast<const AnyExistential*>(value);
    if (any->type != nullptr) {
      debugDescribe(t, anyContents(any), any->type);
      return;
    }
  }
  if (type->kind == kindOptional || type->kind == kindArray || type->kind == kindDictionary ||
      type->kind == kindSet || type->kind == kindTuple) {
    describe(t, value, type);
    return;
  }
  if (is(type, vertex_metadata_String)) {
    debugString(t, *static_cast<const String*>(value));
    return;
  }
  // Swift's debug description is the debugDescription, falling back to
  // the description, and only then to reflection.
  if (debugDescribeConforming(t, value, type))
    return;
  if (describeConforming(t, value, type))
    return;
  if (type->kind == kindStruct && reinterpret_cast<const StructMetadata*>(type)->description != nullptr) {
    describeStruct(t, value, reinterpret_cast<const StructMetadata*>(type), true);
    return;
  }
  if (type->kind == kindEnum) {
    describeEnum(t, value, reinterpret_cast<const StructMetadata*>(type), true);
    return;
  }
  describe(t, value, type);
}

// typeName writes a type's name as `String(describing: T.self)` gives it:
// unqualified, with an Optional, Array, Dictionary or Set written out as
// the generic type it is.
void typeName(Text& t, const Metadata* type) {
  static const struct {
    const FullMetadata* record;
    const char* name;
  } builtins[] = {
      {&vertex_metadata_Bool, "Bool"},     {&vertex_metadata_Int8, "Int8"},
      {&vertex_metadata_UInt8, "UInt8"},   {&vertex_metadata_Int16, "Int16"},
      {&vertex_metadata_UInt16, "UInt16"}, {&vertex_metadata_Int32, "Int32"},
      {&vertex_metadata_UInt32, "UInt32"}, {&vertex_metadata_Float, "Float"},
      {&vertex_metadata_Int, "Int"},       {&vertex_metadata_UInt, "UInt"},
      {&vertex_metadata_Int64, "Int64"},   {&vertex_metadata_UInt64, "UInt64"},
      {&vertex_metadata_Double, "Double"}, {&vertex_metadata_String, "String"},
      {&vertex_metadata_Any, "Any"},
  };
  if (type == nullptr) {
    textString(t, "<unknown>");
    return;
  }
  for (auto& b : builtins) {
    if (is(type, *b.record)) {
      textString(t, b.name);
      return;
    }
  }
  switch (type->kind) {
  case kindOptional:
    textString(t, "Optional<");
    typeName(t, static_cast<const OptionalMetadata*>(type)->payload);
    textByte(t, '>');
    return;
  case kindArray:
    textString(t, "Array<");
    typeName(t, static_cast<const ArrayMetadata*>(type)->element);
    textByte(t, '>');
    return;
  case kindDictionary:
    textString(t, "Dictionary<");
    typeName(t, static_cast<const DictionaryMetadata*>(type)->key);
    textString(t, ", ");
    typeName(t, static_cast<const DictionaryMetadata*>(type)->value);
    textByte(t, '>');
    return;
  case kindSet:
    textString(t, "Set<");
    typeName(t, static_cast<const SetMetadata*>(type)->element);
    textByte(t, '>');
    return;
  case kindTuple: {
    auto* tuple = static_cast<const TupleMetadata*>(type);
    textByte(t, '(');
    for (usize i = 0; i < tuple->numElements; i++) {
      if (i > 0)
        textString(t, ", ");
      typeName(t, tuple->elements[i].type);
    }
    textByte(t, ')');
    return;
  }
  case kindMetatype:
    textString(t, "Metatype");
    return;
  case kindExistential:
    textString(t, "Any");
    return;
  case kindStruct:
  case kindEnum:
  case kindClass:
    if (auto* d = reinterpret_cast<const StructMetadata*>(type)->description) {
      if (const char* name = relativeString(&d->name)) {
        textString(t, name);
        return;
      }
    }
    break;
  }
  textString(t, "<unknown>");
}

// describe writes what `String(describing:)` gives for the value at
// `value`, whose type is `type`.
void describe(Text& t, const void* value, const Metadata* type) {
  // A metatype is described by the type it names.
  if (type->kind == kindMetatype) {
    typeName(t, *static_cast<const Metadata* const*>(value));
    return;
  }
  if (is(type, vertex_metadata_String)) {
    describeString(t, *static_cast<const String*>(value));
    return;
  }
  if (is(type, vertex_metadata_Int) || is(type, vertex_metadata_Int64)) {
    textSigned(t, *static_cast<const i64*>(value));
    return;
  }
  if (is(type, vertex_metadata_UInt) || is(type, vertex_metadata_UInt64)) {
    textUnsigned(t, *static_cast<const u64*>(value));
    return;
  }
  if (is(type, vertex_metadata_Int32)) {
    textSigned(t, *static_cast<const i32*>(value));
    return;
  }
  if (is(type, vertex_metadata_UInt32)) {
    textUnsigned(t, *static_cast<const u32*>(value));
    return;
  }
  if (is(type, vertex_metadata_Int16)) {
    textSigned(t, *static_cast<const short*>(value));
    return;
  }
  if (is(type, vertex_metadata_UInt16)) {
    textUnsigned(t, *static_cast<const u16*>(value));
    return;
  }
  if (is(type, vertex_metadata_Int8)) {
    textSigned(t, *static_cast<const signed char*>(value));
    return;
  }
  if (is(type, vertex_metadata_UInt8)) {
    textUnsigned(t, *static_cast<const u8*>(value));
    return;
  }
  if (is(type, vertex_metadata_Bool)) {
    textString(t, (*static_cast<const u8*>(value) & 1) ? "true" : "false");
    return;
  }
  if (is(type, vertex_metadata_Double)) {
    describeFloat(t, *static_cast<const u64*>(value), 53, 11);
    return;
  }
  if (is(type, vertex_metadata_Float)) {
    describeFloat(t, *static_cast<const u32*>(value), 24, 8);
    return;
  }
  if (type->kind == kindOptional) {
    auto* o = static_cast<const OptionalMetadata*>(type);
    if (optionalIsNone(value, o)) {
      textString(t, "nil");
      return;
    }
    textString(t, "Optional(");
    debugDescribe(t, value, o->payload);
    textByte(t, ')');
    return;
  }
  if (type->kind == kindArray) {
    auto* a = static_cast<const ArrayMetadata*>(type);
    auto* storage = *static_cast<const ArrayStorage* const*>(value);
    auto stride = witnesses(a->element)->stride;
    auto* at = reinterpret_cast<const u8*>(storage) + arrayStorageElements;
    textByte(t, '[');
    for (i64 i = 0; i < storage->count; i++) {
      if (i > 0)
        textString(t, ", ");
      debugDescribe(t, at + static_cast<usize>(i) * stride, a->element);
    }
    textByte(t, ']');
    return;
  }
  if (type->kind == kindDictionary || type->kind == kindSet) {
    // In bucket order, which is the table's and differs between runs as
    // Swift's order does.
    auto* table = *static_cast<const HashTable* const*>(value);
    const Metadata* key = type->kind == kindDictionary
                              ? static_cast<const DictionaryMetadata*>(type)->key
                              : static_cast<const SetMetadata*>(type)->element;
    const Metadata* val = type->kind == kindDictionary ? static_cast<const DictionaryMetadata*>(type)->value : nullptr;
    if (table->count == 0) {
      textString(t, type->kind == kindDictionary ? "[:]" : "[]");
      return;
    }
    auto* base = reinterpret_cast<const u8*>(table);
    const u8* used = base + hashTableUsed;
    usize keyStride = witnesses(key)->stride;
    usize valStride = val != nullptr ? witnesses(val)->stride : 0;
    textByte(t, '[');
    bool first = true;
    for (i64 i = 0; i < table->buckets; i++) {
      if (!used[i])
        continue;
      if (!first)
        textString(t, ", ");
      first = false;
      debugDescribe(t, base + table->keysOffset + static_cast<usize>(i) * keyStride, key);
      if (val != nullptr) {
        textString(t, ": ");
        debugDescribe(t, base + table->valuesOffset + static_cast<usize>(i) * valStride, val);
      }
    }
    textByte(t, ']');
    return;
  }
  if (type->kind == kindTuple) {
    describeTuple(t, value, static_cast<const TupleMetadata*>(type));
    return;
  }
  // A function says only that it is one, as Swift's does.
  if (type->kind == kindFunction) {
    textString(t, "(Function)");
    return;
  }
  if (describeConforming(t, value, type))
    return;
  if (type->kind == kindClass) {
    // Named by its dynamic type where the object says what that is, as
    // Swift names it: `main.Holder`, whatever the static type was.
    auto* object = *static_cast<const HeapObject* const*>(value);
    const Metadata* dynamic = type;
    if (object != nullptr && object->metadata != nullptr && object->metadata->type != nullptr)
      dynamic = object->metadata->type;
    qualifiedName(t, reinterpret_cast<const StructMetadata*>(dynamic)->description);
    return;
  }
  if (type->kind == kindExistential) {
    auto* any = static_cast<const AnyExistential*>(value);
    describe(t, anyContents(any), any->type);
    return;
  }
  if (type->kind == kindStruct) {
    describeStruct(t, value, reinterpret_cast<const StructMetadata*>(type), false);
    return;
  }
  if (type->kind == kindEnum) {
    describeEnum(t, value, reinterpret_cast<const StructMetadata*>(type), false);
    return;
  }
  textString(t, "<unknown>");
}

} // namespace vertex
