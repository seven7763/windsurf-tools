package main

import goruntime "runtime"

func pickTrayIcon(goos string, pngBytes, windowsICO []byte) []byte {
	if goos == "windows" && len(windowsICO) > 0 {
		return windowsICO
	}
	return pngBytes
}

func currentTrayIcon() []byte {
	return pickTrayIcon(goruntime.GOOS, trayIconPNG, trayIconWindowsICO)
}
