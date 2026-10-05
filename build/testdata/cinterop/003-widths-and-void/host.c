#include <stdbool.h>
#include <stdint.h>

extern int32_t vs_narrow(int8_t, int16_t);
extern uint32_t vs_unsigned(uint32_t);
extern int64_t vs_wide(int64_t);
extern double vs_float(double);
extern bool vs_flag(int32_t);
extern void vs_keep(int32_t);

static int32_t recorded = 0;
void c_record(int32_t x) { recorded = x; }

int main(void) {
    if (vs_narrow(-100, 1000) != 900) return 1;
    if (vs_unsigned(0xFFFFFFFFu) != 0u) return 2;
    if (vs_wide(4000000000LL) != 8000000000LL) return 3;
    if (vs_float(2.0) != 3.0) return 4;
    if (!vs_flag(1)) return 5;
    if (vs_flag(-1)) return 6;
    vs_keep(17);
    if (recorded != 51) return 7;
    return 42;
}
