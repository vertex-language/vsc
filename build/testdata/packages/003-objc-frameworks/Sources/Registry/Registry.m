#import <Foundation/Foundation.h>
#import <CoreGraphics/CoreGraphics.h>
#include "Registry.h"

// Strong under ARC: the dictionary outlives the pool it was created in.
static NSMutableDictionary<NSString*, NSValue*>* rects;

static void ensure(void) {
    if (rects == nil) {
        @autoreleasepool {
            rects = [NSMutableDictionary dictionary];
        }
    }
}

void registry_add(const char* name, double width, double height) {
    ensure();
    @autoreleasepool {
        NSRect r = NSMakeRect(0, 0, width, height);
        rects[[NSString stringWithUTF8String:name]] = [NSValue valueWithRect:r];
    }
}

int32_t registry_count(void) {
    ensure();
    return (int32_t)[rects count];
}

double registry_area(const char* name) {
    ensure();
    NSValue* v = rects[[NSString stringWithUTF8String:name]];
    if (v == nil)
        return -1;
    NSRect r = [v rectValue];
    return r.size.width * r.size.height;
}

double registry_union_width(void) {
    ensure();
    CGRect all = CGRectNull;
    for (NSValue* v in [rects allValues])
        all = CGRectUnion(all, NSRectToCGRect([v rectValue]));
    return CGRectIsNull(all) ? 0 : CGRectGetWidth(all);
}
