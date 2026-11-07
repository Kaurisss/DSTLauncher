//go:build desktop && windows

package main

import (
	"fmt"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"

	_ "dst-windows/internal/desktopresources"

	webview2 "github.com/jchv/go-webview2"
	"golang.org/x/sys/windows"
)

func init() {
	// WebView2 的窗口及消息循环必须运行在同一个系统线程。
	runtime.LockOSThread()
}

func runPanelWindow(url, workDir string, stopped <-chan struct{}) error {
	// 库在初始化完成前就显示窗口；延后 WM_CLOSE，避免其异步回调访问已销毁的窗口。
	user32 := windows.NewLazySystemDLL("user32.dll")
	dpi, _, _ := user32.NewProc("GetDpiForSystem").Call()
	var work desktopRect
	if ok, _, err := user32.NewProc("SystemParametersInfoW").Call(0x0030, 0, uintptr(unsafe.Pointer(&work)), 0); ok == 0 {
		return fmt.Errorf("读取屏幕工作区失败: %w", err)
	}
	width, height := desktopWindowSize(int(dpi), int(work.right-work.left), int(work.bottom-work.top))
	callNext := user32.NewProc("CallNextHookEx")
	closeRequested := false
	type windowMessage struct {
		window  uintptr
		message uint32
		wparam  uintptr
		lparam  uintptr
		time    uint32
		x, y    int32
	}
	callback := syscall.NewCallback(func(code int32, wparam, lparam uintptr) uintptr {
		if code >= 0 && wparam == 1 { // PM_REMOVE
			msg := (*windowMessage)(unsafe.Pointer(lparam))
			if msg.message == 0x0010 { // WM_CLOSE
				closeRequested = true
				msg.message = 0 // WM_NULL
			}
		}
		result, _, _ := callNext.Call(0, uintptr(code), wparam, lparam)
		return result
	})
	hook, _, hookErr := user32.NewProc("SetWindowsHookExW").Call(3, callback, 0, uintptr(windows.GetCurrentThreadId()))
	if hook == 0 {
		return fmt.Errorf("监听窗口初始化消息失败: %w", hookErr)
	}
	view := webview2.NewWithOptions(webview2.WebViewOptions{
		DataPath:  filepath.Join(workDir, "webview2"),
		AutoFocus: true,
		WindowOptions: webview2.WindowOptions{
			Title: "DST 开服管理系统", Width: uint(width), Height: uint(height), IconId: 1, Center: true,
		},
	})
	user32.NewProc("UnhookWindowsHookEx").Call(hook)
	if view == nil {
		return fmt.Errorf("无法创建 WebView2 窗口，请安装 Microsoft Edge WebView2 Runtime")
	}
	defer view.Destroy()
	if closeRequested {
		return nil
	}
	view.SetSize(min(900*int(dpi)/96, width), min(600*int(dpi)/96, height), webview2.HintMin)
	// WebView2 按窗口 DPI 渲染；跨屏时应用 Windows 建议的物理像素尺寸。
	hwnd := uintptr(view.Window())
	setWindowPos := user32.NewProc("SetWindowPos")
	setWindowPos.Call(hwnd, 0, uintptr(work.left+(work.right-work.left-int32(width))/2), uintptr(work.top+(work.bottom-work.top-int32(height))/2), 0, 0, 0x0015)
	callWindowProc := user32.NewProc("CallWindowProcW")
	setWindowLong := user32.NewProc("SetWindowLongPtrW")
	if unsafe.Sizeof(uintptr(0)) == 4 {
		setWindowLong = user32.NewProc("SetWindowLongW")
	}
	var previousProc uintptr
	windowProc := syscall.NewCallback(func(window uintptr, message uint32, wparam, lparam uintptr) uintptr {
		if message == 0x02E0 { // WM_DPICHANGED
			rect := (*desktopRect)(unsafe.Pointer(lparam))
			newDPI := int(wparam & 0xffff)
			view.SetSize(min(900*newDPI/96, int(rect.right-rect.left)), min(600*newDPI/96, int(rect.bottom-rect.top)), webview2.HintMin)
			setWindowPos.Call(window, 0, uintptr(rect.left), uintptr(rect.top), uintptr(rect.right-rect.left), uintptr(rect.bottom-rect.top), 0x0014)
			return 0
		}
		result, _, _ := callWindowProc.Call(previousProc, window, uintptr(message), wparam, lparam)
		return result
	})
	previousProc, _, err := setWindowLong.Call(hwnd, ^uintptr(3), windowProc) // GWLP_WNDPROC = -4
	if previousProc == 0 {
		return fmt.Errorf("设置窗口 DPI 处理失败: %w", err)
	}
	view.Navigate(url)
	fmt.Printf("Desktop window ready (DPI=%d)\n", dpi)
	closed := make(chan struct{})
	defer close(closed)
	go func() {
		select {
		case <-stopped:
			view.Dispatch(view.Terminate)
		case <-closed:
		}
	}()
	view.Run()
	return nil
}

type desktopRect struct {
	left, top, right, bottom int32
}

func desktopWindowSize(dpi, workWidth, workHeight int) (int, int) {
	margin := 32 * dpi / 96
	return min(1280*dpi/96, workWidth-margin), min(860*dpi/96, workHeight-margin)
}
