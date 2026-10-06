//go:build windows && cgo

package main

/*
#cgo windows LDFLAGS: -lgdi32 -luser32
#include <windows.h>
#include <wchar.h>

static HHOOK background_hook;
static HBRUSH background_brush;
static LRESULT CALLBACK background_proc(int code, WPARAM wp, LPARAM lp) {
    if (code == HCBT_CREATEWND) {
        wchar_t name[64];
        if (GetClassNameW((HWND)wp, name, 64) && wcscmp(name, L"webview") == 0) {
            if (!background_brush) background_brush = CreateSolidBrush(RGB(20, 26, 35));
            SetClassLongPtrW((HWND)wp, GCLP_HBRBACKGROUND, (LONG_PTR)background_brush);
        }
    }
    return CallNextHookEx(background_hook, code, wp, lp);
}
static void start_background_hook(void) {
    background_hook = SetWindowsHookExW(WH_CBT, background_proc, NULL, GetCurrentThreadId());
}
static void stop_background_hook(void) {
    if (background_hook) UnhookWindowsHookEx(background_hook);
    background_hook = NULL;
}
*/
import "C"

// The thread-local hook paints the native host before webview_go first shows it.
// The class owns the brush for the lifetime of this process.
func prepareWindowBackground() func() {
	C.start_background_hook()
	return func() { C.stop_background_hook() }
}
