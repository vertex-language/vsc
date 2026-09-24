#pragma once
#include <stdint.h>

void    registry_add(const char* name, double width, double height);
int32_t registry_count(void);
double  registry_area(const char* name);
// The union of every rectangle, through CoreGraphics.
double  registry_union_width(void);
