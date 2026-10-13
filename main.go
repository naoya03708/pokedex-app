package main

import (
	"log"

	"github.com/egoist/mygo"
	mui "github.com/egoist/mygo/ui"

	"github.com/naoya03708/pokedex-app/internal/app"
	"github.com/naoya03708/pokedex-app/internal/pokeapi"
	"github.com/naoya03708/pokedex-app/internal/ui"
)

func main() {
	cacheDir, err := pokeapi.DefaultCacheDir()
	if err != nil {
		// Without a cache directory the app still works, but fetches every time.
		log.Printf("disk cache disabled: %v", err)
		cacheDir = ""
	}
	client := pokeapi.New(pokeapi.Options{CacheDir: cacheDir})

	mygo.App.WhenReady(func() {
		var win *mygo.Window
		// Background fetches publish their results on the UI thread.
		m := app.New(client, func(fn func()) { win.Update(fn) })
		win = mygo.NewWindow(mygo.WindowOptions{
			Title:     "Pokedex",
			Width:     900,
			Height:    600,
			MinWidth:  640,
			MinHeight: 420,
			// Opens where the user left it last time.
			StateKey: "main",
			// The window shows the interface MyGo draws, not a web page.
			Content: mui.View(ui.New(m).Build),
		})
		// Closing the window cancels every running fetch. OnClosed runs on the
		// main thread, which is where the model lives.
		win.OnClosed(m.Close)
		m.Start()
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
