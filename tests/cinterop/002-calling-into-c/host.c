extern int vs_via_c(int);
extern int vs_biggest(int, int, int);
extern int vs_round_trip(int);

int c_double(int x) { return x * 2; }
int c_max(int a, int b) { return a > b ? a : b; }

int main(void) {
    if (vs_via_c(21) != 43) return 1;
    if (vs_biggest(3, 9, 5) != 9) return 2;
    if (vs_biggest(-1, -7, -3) != -1) return 3;
    if (vs_round_trip(5) != 20) return 4;
    return 42;
}
