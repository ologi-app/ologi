//go:build darwin

package sourceapp

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework AppKit -framework Foundation
#import <AppKit/AppKit.h>
#import <Foundation/Foundation.h>

static const char* frontmostAppName() {
    @autoreleasepool {
        NSRunningApplication *app = [[NSWorkspace sharedWorkspace] frontmostApplication];
        if (app == nil) return NULL;
        NSString *name = app.localizedName ?: @"";
        return strdup([name UTF8String]);
    }
}

static const char* frontmostAppBundleID() {
    @autoreleasepool {
        NSRunningApplication *app = [[NSWorkspace sharedWorkspace] frontmostApplication];
        if (app == nil) return NULL;
        NSString *bid = app.bundleIdentifier ?: @"";
        return strdup([bid UTF8String]);
    }
}
*/
import "C"

import "unsafe"

// appInfo returns (localizedName, bundleID). Empty strings on failure.
func appInfo() (name, bundleID string) {
	cn := C.frontmostAppName()
	if cn != nil {
		name = C.GoString(cn)
		C.free(unsafe.Pointer(cn))
	}
	cb := C.frontmostAppBundleID()
	if cb != nil {
		bundleID = C.GoString(cb)
		C.free(unsafe.Pointer(cb))
	}
	return
}

func detectImpl() string {
	name, bundleID := appInfo()
	if name == "" {
		return ""
	}
	if host := browserTabHost(bundleID); host != "" {
		return name + " / " + host
	}
	return name
}
