package main

import (
	"net/http"

	. "maragu.dev/gomponents"
	. "maragu.dev/gomponents/components"
	. "maragu.dev/gomponents/html"
)

func Navbar(authenticated bool, currentPath string) Node {
	return Nav(
		NavbarLink("/", "Home", currentPath),
		NavbarLink("/about", "About", currentPath),
		If(authenticated, NavbarLink("/profile", "Profile", currentPath)),
	)
}

func NavbarLink(href, name, currentPath string) Node {
	return A(Href(href), Classes{"is-active": currentPath == href}, Text(name))
}

func Page(currentPath string) Node {
	return HTML(
		Head(TitleEl(Text("My Site"))),
		Body(
			Navbar(false, currentPath),
			H1(Text("Hello!")),
		),
	)
}

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_ = Page(r.URL.Path).Render(w)
	})

	http.ListenAndServe(":8080", nil)
}
