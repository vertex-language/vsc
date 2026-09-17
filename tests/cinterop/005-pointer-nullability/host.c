#include <stdbool.h>
#include <stdint.h>
#include <stdlib.h>

extern int32_t vs_or_else(const int32_t *, int32_t);
extern bool vs_write_if(int32_t *, int32_t);
extern bool vs_is_null(const void *);
extern int32_t vs_through(bool);
extern int32_t vs_via_raw(int32_t *);

static int32_t cell = 5;
int32_t *c_maybe(bool want_null) { return want_null ? NULL : &cell; }

int main(void) {
    int32_t x = 7;
    if (vs_or_else(&x, -1) != 7) return 1;
    if (vs_or_else(NULL, -1) != -1) return 2;

    if (!vs_write_if(&x, 11)) return 3;
    if (x != 11) return 4;
    if (vs_write_if(NULL, 12)) return 5;

    if (!vs_is_null(NULL)) return 6;
    if (vs_is_null(&x)) return 7;

    if (vs_through(true) != -1) return 8;
    if (vs_through(false) != 5) return 9;

    if (vs_via_raw(&x) != 11) return 10;
    return 42;
}
