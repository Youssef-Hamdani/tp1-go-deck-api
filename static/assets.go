package static
import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed img/*.svg
var assets embed.FS

func Handler() http.Handler {
	sub, err := fs.Sub(assets, "img")
	if err != nil {
		panic(err)
	}
	return http.StripPrefix("/static/img/", http.FileServer(http.FS(sub)))
}
