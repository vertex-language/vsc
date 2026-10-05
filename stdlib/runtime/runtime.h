// Every target-independent part of the runtime, as one translation
// unit: each file sees the ones before it. A platform unit includes this
// and then says how it reaches its operating system.
#pragma once

#include "heap.cpp"
#include "metadata.cpp"
#include "generic.cpp"
#include "string.cpp"
#include "array.cpp"
#include "conformance.cpp"
#include "describe.cpp"
#include "collections.cpp"
#include "print.cpp"
#include "error.cpp"
#include "task.cpp"
