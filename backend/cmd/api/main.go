package main

import (
	"backend/internal/config"
)

func main() {
	r := config.MountRoutes()
	r.Run(":8080")
}