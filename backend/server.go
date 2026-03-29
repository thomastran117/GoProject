package main

import (
	"backend/internal/application"
)

func main() {
	r := config.MountRoutes()
	r.Run(":8080")
}