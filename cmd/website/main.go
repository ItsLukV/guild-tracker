package main

import (
	"github.com/gin-gonic/gin"
	g "maragu.dev/gomponents"
	hx "maragu.dev/gomponents-htmx"
	. "maragu.dev/gomponents/html"
)

var likes = 0

func LikeButton(count int) g.Node {
	return Button(
		ID("like-btn"),
		hx.Post("/like"),     // click sends POST /like
		hx.Swap("outerHTML"), // replace this button with the response
		g.Textf("👍 %d", count),
	)
}

func Page() g.Node {
	return HTML(
		Head(
			// load htmx
			Script(Src("https://unpkg.com/htmx.org@2")),
		),
		Body(
			H1(g.Text("My Blog")),
			LikeButton(likes),
		),
	)
}

func main() {
	r := gin.Default()

	r.GET("/", func(c *gin.Context) {
		Page().Render(c.Writer)
	})

	r.POST("/like", func(c *gin.Context) {
		likes++
		LikeButton(likes).Render(c.Writer)
	})

	r.Run(":8080")
}
