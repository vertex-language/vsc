// The runtime's collections exercised directly: Array mutation with copy
// on write, and the hash table under insertion, lookup, replacement and
// removal, checked against a plain array kept beside it. Prints nothing
// when every check holds; build/runtime_test.go runs it.
#include "runtime.h"
#include "platform.h"

using namespace vertex;

extern "C" {
extern const FullMetadata vertex_metadata_Int, vertex_metadata_String;
}

namespace {

u64 failures = 0;
Text report;

void check(bool ok, const char* what, i64 n) {
  if (ok)
    return;
  failures++;
  textString(report, what);
  textByte(report, ' ');
  textSigned(report, n);
  textByte(report, '\n');
}

const Metadata* intType() { return &vertex_metadata_Int.metadata; }
const Metadata* stringType() { return &vertex_metadata_String.metadata; }

// The Optional<Int> record a compiler would emit: the value witness table
// the runtime's generic optional witnesses make, then the metadata.
const ValueWitnessTable intOptionalWitnesses = {
    vertex_vw_optional_copy, vertex_vw_optional_destroy, vertex_vw_optional_copy,
    vertex_vw_optional_assign_copy, vertex_vw_take, vertex_vw_optional_assign_take,
    vertex_vw_get_enum_tag, vertex_vw_store_enum_tag, 9, 16, 7, 0};

struct OptionalRecord {
  const ValueWitnessTable* vwt;
  OptionalMetadata         metadata;
};

const OptionalRecord intOptionalRecord = {
    &intOptionalWitnesses, {{kindOptional}, &vertex_metadata_Int.metadata, 8, 1, 1}};
const OptionalMetadata& intOptional = intOptionalRecord.metadata;

struct IntOpt {
  i64 value;
  u8  tag;
  u8  pad[7];
};

String numbered(i64 n) {
  Text t;
  textInit(t);
  textString(t, "key number ");
  textSigned(t, n);
  String s = makeString(t.bytes, t.count);
  textFree(t);
  return s;
}

void arrays() {
  ArrayStorage* a = vertex_array_allocate(0, intType()).array;
  for (i64 i = 0; i < 1000; i++) {
    i64 v = i * 3;
    vertex_array_append(&a, &v, intType());
  }
  check(a->count == 1000, "append count", a->count);

  // A second reference, then a write: the write copies.
  ArrayStorage* shared = a;
  vertex_retain(&a->header);
  i64 seven = 7;
  vertex_array_assign(&a, 10, &seven, intType());
  check(a != shared, "assign copies shared storage", 0);
  check(*reinterpret_cast<i64*>(vertex_array_element(10, shared, intType())) == 30, "original untouched", 10);
  check(*reinterpret_cast<i64*>(vertex_array_element(10, a, intType())) == 7, "copy written", 10);
  vertex_release(&shared->header);

  i64 front = -1;
  vertex_array_insert(&a, &front, 0, intType());
  i64 out = 0;
  vertex_array_remove_at(&a, 1, &out, intType());
  check(out == 0 && a->count == 1000, "insert then remove", out);
  vertex_array_remove_last(&a, &out, intType());
  check(out == 999 * 3, "remove last", out);
  i64 probe = 21;
  check(vertex_array_contains(a, &probe, intType()), "contains", probe);

  IntOpt first{};
  vertex_array_first(a, &first, &intOptional);
  check(first.tag == 0 && first.value == -1, "first", first.value);
  vertex_release(&a->header);

  ArrayStorage* empty = vertex_array_allocate(0, intType()).array;
  IntOpt none{};
  vertex_array_last(empty, &none, &intOptional);
  check(none.tag == 1, "last of empty", none.tag);
  IntOpt popped{};
  vertex_array_pop_last(&empty, &popped, &intOptional);
  check(popped.tag == 1 && empty->count == 0, "pop last of empty", popped.tag);

  // Strings, whose copies and moves go through their witnesses.
  ArrayStorage* strings = vertex_array_allocate(0, stringType()).array;
  for (i64 i = 0; i < 300; i++) {
    String s = numbered(i);
    vertex_array_append(&strings, &s, stringType());
  }
  ArrayStorage* other = strings;
  vertex_retain(&other->header);
  vertex_array_append_contents(&strings, other, stringType());
  check(strings->count == 600, "append contents", strings->count);
  vertex_release(&other->header);
  vertex_release(&strings->header);
}

void dictionaries() {
  HashTable* d = vertex_hash_table_empty();
  const i64 n = 5000;
  for (i64 i = 0; i < n; i++) {
    String key = numbered(i);
    IntOpt value{i * i, 0, {}};
    vertex_dictionary_set(&d, &key, &value, stringType(), &intOptional);
    vertex_string_release(key.object);
  }
  check(d->count == n, "dictionary count", d->count);

  // A shared table is copied before it changes.
  HashTable* snapshot = d;
  vertex_retain(&d->header);

  for (i64 i = 0; i < n; i += 2) {
    String key = numbered(i);
    IntOpt gone{};
    vertex_dictionary_remove(&d, &key, &gone, stringType(), &intOptional);
    check(gone.tag == 0 && gone.value == i * i, "removed value", i);
    vertex_string_release(key.object);
  }
  check(d->count == n / 2, "count after removal", d->count);
  check(snapshot->count == n, "snapshot untouched", snapshot->count);

  for (i64 i = 0; i < n; i++) {
    String key = numbered(i);
    IntOpt got{};
    vertex_dictionary_get(d, &key, &got, &intOptional);
    bool present = got.tag == 0;
    check(present == (i % 2 == 1), "lookup after removal", i);
    if (present)
      check(got.value == i * i, "value after removal", i);
    IntOpt old{};
    vertex_dictionary_get(snapshot, &key, &old, &intOptional);
    check(old.tag == 0 && old.value == i * i, "snapshot lookup", i);
    vertex_string_release(key.object);
  }

  // Assigning nil removes; assigning over a key replaces.
  String key = numbered(1);
  IntOpt nil{0, 1, {}};
  vertex_dictionary_set(&d, &key, &nil, stringType(), &intOptional);
  IntOpt replaced{42, 0, {}};
  String three = numbered(3);
  vertex_dictionary_set(&d, &three, &replaced, stringType(), &intOptional);
  IntOpt got{};
  vertex_dictionary_get(d, &three, &got, &intOptional);
  check(got.tag == 0 && got.value == 42, "replaced", got.value);
  vertex_dictionary_get(d, &key, &got, &intOptional);
  check(got.tag == 1, "nil removed", got.tag);

  // `d[k, default: v]`: the value where the key is, the default where not.
  i64 fallback = -5, answer = 0;
  vertex_dictionary_get_default(d, &three, &fallback, &answer, intType());
  check(answer == 42, "default of present key", answer);
  vertex_dictionary_get_default(d, &key, &fallback, &answer, intType());
  check(answer == -5, "default of missing key", answer);
  vertex_string_release(key.object);
  vertex_string_release(three.object);

  // Canonically equal keys are one key.
  String composed = makeString(reinterpret_cast<const u8*>("\xC3\xA9"), 2);
  String decomposed = makeString(reinterpret_cast<const u8*>("e\xCC\x81"), 3);
  IntOpt one{1, 0, {}};
  vertex_dictionary_set(&d, &composed, &one, stringType(), &intOptional);
  vertex_dictionary_get(d, &decomposed, &got, &intOptional);
  check(got.tag == 0 && got.value == 1, "canonical key", got.value);

  vertex_release(&d->header);
  vertex_release(&snapshot->header);
}

void sets() {
  HashTable* s = vertex_hash_table_empty();
  for (i64 i = 0; i < 2000; i++) {
    i64 v = i % 700;
    vertex_set_insert(&s, &v, intType());
  }
  check(s->count == 700, "set count", s->count);
  i64 in = 699, out = 700;
  check(vertex_set_contains(s, &in, intType()), "set contains", in);
  check(!vertex_set_contains(s, &out, intType()), "set lacks", out);
  IntOpt removed{};
  vertex_set_remove(&s, &in, &removed, &intOptional);
  check(removed.tag == 0 && removed.value == 699 && s->count == 699, "set remove", removed.value);

  // A for-in's walk: every element once, whatever the bucket order.
  i64 seen = 0, sum = 0;
  for (i64 at = vertex_hash_table_next(s, 0); at >= 0; at = vertex_hash_table_next(s, at + 1)) {
    i64 v = -1;
    vertex_hash_table_key_at(s, at, &v, intType());
    seen++;
    sum += v;
  }
  check(seen == 699 && sum == 698 * 699 / 2, "walk", sum);
  check(vertex_hash_table_next(vertex_hash_table_empty(), 0) == -1, "walk of empty", 0);
  vertex_release(&s->header);
}

} // namespace

int main() {
  textInit(report);
  arrays();
  dictionaries();
  sets();
  vertex_pal_write(1, report.bytes, report.count);
  return failures == 0 ? 0 : 1;
}
