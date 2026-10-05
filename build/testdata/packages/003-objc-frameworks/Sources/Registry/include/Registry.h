#pragma once
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif


void    registry_add(const char* name, double width, double height);
int32_t registry_count(void);
double  registry_area(const char* name);
// The union of every rectangle, through CoreGraphics.
double  registry_union_width(void);

#ifdef __cplusplus
}
#endif
