#include <stdbool.h>
#include <stdint.h>
#include <stdlib.h>

extern int32_t vs_read(const int32_t *);
extern void vs_write(int32_t *, int32_t);
extern void vs_bump(int32_t *);
extern void vs_swap(int32_t *, int32_t *);
extern void vs_divmod(int32_t, int32_t, int32_t *, int32_t *);
extern int32_t vs_round_trip(int32_t);
extern bool vs_same(const int32_t *, const int32_t *);

int32_t *c_alloc_int(int32_t v) { int32_t *p = malloc(sizeof *p); *p = v; return p; }
void c_free_int(int32_t *p) { free(p); }

int main(void) {
    int32_t x = 7, y = 9;
    if (vs_read(&x) != 7) return 1;
    vs_write(&x, 42);
    if (x != 42) return 2;
    vs_bump(&x);
    if (x != 43) return 3;
    vs_swap(&x, &y);
    if (x != 9 || y != 43) return 4;

    int32_t q = 0, r = 0;
    vs_divmod(17, 5, &q, &r);
    if (q != 3 || r != 2) return 5;

    if (vs_round_trip(5) != 15) return 6;

    if (!vs_same(&x, &x)) return 7;
    if (vs_same(&x, &y)) return 8;

    /* A pointer into an array walks with C's arithmetic, and each
       element is read through the same entry point. */
    int32_t a[3] = {10, 20, 30};
    if (vs_read(&a[0]) != 10) return 9;
    if (vs_read(&a[2]) != 30) return 10;
    vs_write(&a[1], 99);
    if (a[1] != 99) return 11;
    return 42;
}
