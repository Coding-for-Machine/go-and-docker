package main

import (
	"go-leetcode/app/docker"

	"github.com/gofiber/fiber/v2"
)

func Home(c *fiber.Ctx) error {
	return c.SendString("Home Page")
}
func main() {
	app := fiber.New()
	app.Get("/", Home)
	app.Post("/api/run/", docker.DockerRun)
	app.Listen(":3000")
}
