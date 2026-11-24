package main

import (
	"log"

	"app/internal/config"
	"app/internal/database"
	"app/internal/server"
)

func main() {
	cfg := config.LoadConfig()

	db, err := database.Init(cfg)
	if err != nil {
		log.Fatalf("gagal menginisialisasi database: %v", err)
	}

	router := server.NewRouter(cfg, db)
	if err := router.Run(":" + cfg.AppPort); err != nil {
		log.Fatalf("gagal menjalankan server: %v", err)
	}
}
