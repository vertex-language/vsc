#include <stdio.h>

extern int vs_add(int, int);
extern int vs_fib(int);
extern int vs_sum(int);

int main(void) {
    if (vs_add(3, 4) != 7) return 1;
    if (vs_add(-3, 4) != 1) return 2;
    if (vs_fib(10) != 55) return 3;
    if (vs_fib(0) != 0) return 4;
    if (vs_sum(10) != 55) return 5;
    (void)printf;
    return 42;
}
