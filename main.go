package main

import (
	"embed"
	"log"
	"os"

	"github.com/kardianos/service"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var trayIconPNG []byte

//go:embed build/windows/icon.ico
var trayIconWindowsICO []byte

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "install", "uninstall", "start", "stop", "restart":
			if err := runServiceControl(os.Args[1]); err != nil {
				log.Fatal(err)
			}
			return
		}
	}
	if !service.Interactive() {
		if err := runHeadlessDaemon(); err != nil {
			log.Fatal(err)
		}
		return
	}

	silent := false
	for _, a := range os.Args[1:] {
		if a == "-silent" || a == "--silent" || a == "--silent-start" {
			silent = true
			break
		}
	}

	app := NewApp()
	app.SetSilentFromFlag(silent)

	err := wails.Run(&options.App{
		Title:     "Windsurf Tools",
		Width:     1100,
		Height:    750,
		MinWidth:  800,
		MinHeight: 560,
		// 标准 Win32 边框窗口：系统标题栏（最小化 / 最大化 / 关闭）、边缘拖拽缩放
		Frameless:        false,
		DisableResize:    false,
		WindowStartState: options.Normal,
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
		},
		Mac: &mac.Options{
			TitleBar:             mac.TitleBarHiddenInset(),
			WebviewIsTransparent: true,
			WindowIsTranslucent:  false,
		},
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId:               "com.shaoyu521.windsurf-tools-wails",
			OnSecondInstanceLaunch: app.onSecondInstanceLaunch,
		},
		BackgroundColour: &options.RGBA{R: 24, G: 24, B: 30, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		OnBeforeClose:    app.onBeforeClose,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
